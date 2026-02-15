package webx

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"codeberg.org/readeck/go-readability/v2"
	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/bytedance/sonic"
	"github.com/cloudwego/eino/schema"

	"github.com/tgifai/friday/internal/pkg/logs"
)

const (
	fetchTimeout    = 30 * time.Second
	fetchMaxChars   = 50000
	fetchMaxRedirs  = 5
	fetchUserAgent  = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	fetchMaxBodyMiB = 5 // max response body to read (MiB)
)

type FetchTool struct {
	client *http.Client
}

func NewFetchTool() *FetchTool {
	return &FetchTool{
		client: &http.Client{
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= fetchMaxRedirs {
					return fmt.Errorf("too many redirects (max %d)", fetchMaxRedirs)
				}
				if isPrivateHost(req.URL.Hostname()) {
					return fmt.Errorf("redirect to private address blocked")
				}
				return nil
			},
			Timeout:   fetchTimeout,
			Transport: &http.Transport{ForceAttemptHTTP2: true},
		},
	}
}

func (t *FetchTool) Name() string { return "web_fetch" }

func (t *FetchTool) Description() string {
	return "Fetch a URL and extract its main content as markdown. Supports HTML pages (via readability extraction), JSON endpoints, and Cloudflare Browser Rendering for JS-heavy pages."
}

func (t *FetchTool) ToolInfo() *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: t.Name(),
		Desc: t.Description(),
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"url": {
				Type:     schema.String,
				Desc:     "The URL to fetch (must be http or https)",
				Required: true,
			},
			"max_chars": {
				Type: schema.Integer,
				Desc: "Maximum characters to return (default 50000)",
			},
			"render_js": {
				Type: schema.Boolean,
				Desc: "Use Cloudflare Browser Rendering for JS-heavy pages (requires CLOUDFLARE_API_TOKEN and CLOUDFLARE_ACCOUNT_ID env vars)",
			},
		}),
	}
}

type fetchResult struct {
	URL       string `json:"url"`
	Title     string `json:"title,omitempty"`
	Status    int    `json:"status"`
	Length    int    `json:"length"`
	Truncated bool   `json:"truncated"`
	Content   string `json:"content"`
}

func (t *FetchTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	rawURL, _ := args["url"].(string)
	if rawURL == "" {
		return nil, fmt.Errorf("url is required")
	}

	maxChars := fetchMaxChars
	if v, ok := args["max_chars"].(float64); ok && v > 0 {
		maxChars = int(v)
	}

	renderJS, _ := args["render_js"].(bool)

	// Validate URL.
	parsed, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("only http and https URLs are allowed")
	}
	if isPrivateHost(parsed.Hostname()) {
		return nil, fmt.Errorf("access to private/internal addresses is not allowed")
	}

	// If render_js is requested and Cloudflare renderer is available, use it.
	if renderJS {
		if cfr := getCloudflareRenderer(); cfr != nil {
			return t.executeCloudflare(ctx, cfr, rawURL, maxChars)
		}
		logs.CtxWarn(ctx, "[tool:web_fetch] render_js requested but CLOUDFLARE_API_TOKEN/CLOUDFLARE_ACCOUNT_ID not set, falling back to direct fetch")
	}

	return t.executeDirect(ctx, rawURL, parsed, maxChars)
}

// executeCloudflare uses the Cloudflare Browser Rendering /markdown endpoint.
func (t *FetchTool) executeCloudflare(ctx context.Context, cfr *cloudflareRenderer, rawURL string, maxChars int) (interface{}, error) {
	title, markdown, err := cfr.RenderMarkdown(ctx, rawURL)
	if err != nil {
		return nil, fmt.Errorf("cloudflare render: %w", err)
	}

	content := markdown
	truncated := false
	if len(content) > maxChars {
		content = content[:maxChars]
		truncated = true
	}

	result := fetchResult{
		URL:       rawURL,
		Title:     title,
		Status:    200,
		Length:    len(content),
		Truncated: truncated,
		Content:   content,
	}

	logs.CtxInfo(ctx, "[tool:web_fetch] %s via cloudflare (%d chars, truncated=%v)", rawURL, result.Length, truncated)

	out, _ := sonic.MarshalString(result)
	return out, nil
}

// executeDirect fetches the URL directly with content negotiation.
func (t *FetchTool) executeDirect(ctx context.Context, rawURL string, parsed *url.URL, maxChars int) (interface{}, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", fetchUserAgent)
	// Content negotiation: prefer markdown (Cloudflare-fronted sites), then HTML/JSON.
	req.Header.Set("Accept", "text/markdown, text/html, application/json, */*")

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}
	defer resp.Body.Close()

	// Limit body size.
	body, err := io.ReadAll(io.LimitReader(resp.Body, fetchMaxBodyMiB*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	ctype := resp.Header.Get("Content-Type")
	var title, content string

	switch {
	case strings.Contains(ctype, "text/markdown"):
		// Server returned markdown directly (e.g. Cloudflare content negotiation).
		content = string(body)

	case strings.Contains(ctype, "application/json"):
		// Pretty-print JSON.
		var js interface{}
		if err := sonic.Unmarshal(body, &js); err == nil {
			if pretty, err := sonic.MarshalIndent(js, "", "  "); err == nil {
				content = string(pretty)
			}
		}
		if content == "" {
			content = string(body)
		}

	case strings.Contains(ctype, "text/html") || looksLikeHTML(body):
		title, content = extractReadable(body, parsed)

	default:
		content = string(body)
	}

	truncated := false
	if len(content) > maxChars {
		content = content[:maxChars]
		truncated = true
	}

	result := fetchResult{
		URL:       resp.Request.URL.String(),
		Title:     title,
		Status:    resp.StatusCode,
		Length:    len(content),
		Truncated: truncated,
		Content:   content,
	}

	logs.CtxInfo(ctx, "[tool:web_fetch] %s (%d chars, truncated=%v)", rawURL, result.Length, truncated)

	out, _ := sonic.MarshalString(result)
	return out, nil
}

// extractReadable uses go-readability to extract the main content from HTML,
// then converts the cleaned HTML to markdown.
func extractReadable(body []byte, pageURL *url.URL) (title, markdown string) {
	article, err := readability.FromReader(bytes.NewReader(body), pageURL)
	if err != nil {
		// Fallback: convert raw HTML to markdown.
		md, _ := htmltomarkdown.ConvertString(string(body))
		return "", md
	}

	title = article.Title()

	// Render cleaned HTML from readability, then convert to markdown.
	var buf bytes.Buffer
	if err := article.RenderHTML(&buf); err != nil {
		// Fallback to plain text.
		var tbuf bytes.Buffer
		_ = article.RenderText(&tbuf)
		return title, tbuf.String()
	}

	md, err := htmltomarkdown.ConvertString(buf.String())
	if err != nil {
		return title, buf.String() // return HTML if markdown conversion fails
	}

	if title != "" {
		markdown = "# " + title + "\n\n" + md
	} else {
		markdown = md
	}
	return title, markdown
}

func looksLikeHTML(body []byte) bool {
	prefix := strings.TrimSpace(strings.ToLower(string(body[:min(256, len(body))])))
	return strings.HasPrefix(prefix, "<!doctype") || strings.HasPrefix(prefix, "<html")
}

// isPrivateHost checks whether a hostname resolves to a private/loopback address.
func isPrivateHost(host string) bool {
	// Check well-known private hostnames first.
	if host == "localhost" || host == "metadata.google.internal" {
		return true
	}

	ips, err := net.LookupIP(host)
	if err != nil {
		// If DNS fails, check if it's a raw IP.
		ip := net.ParseIP(host)
		if ip == nil {
			return false
		}
		return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
	}

	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
			return true
		}
	}
	return false
}
