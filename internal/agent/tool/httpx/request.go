package httpx

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/cloudwego/eino/schema"

	"github.com/tgifai/friday/internal/pkg/logs"
	pkgutils "github.com/tgifai/friday/internal/pkg/utils"
)

const (
	defaultTimeout  = 30 * time.Second
	maxTimeout      = 120 * time.Second
	maxRedirects    = 5
	maxBodyMiB      = 5
	maxResponseChar = 50000
	userAgent       = "friday-httpx/1.0"
)

// isPrivateHost is the SSRF check function. Tests may override this.
var isPrivateHost = pkgutils.IsPrivateHost

var allowedMethods = map[string]bool{
	http.MethodGet:    true,
	http.MethodPost:   true,
	http.MethodPut:    true,
	http.MethodPatch:  true,
	http.MethodDelete: true,
}

// RequestTool lets the agent make arbitrary HTTP requests to external APIs.
type RequestTool struct{}

func NewRequestTool() *RequestTool {
	return &RequestTool{}
}

func (t *RequestTool) Name() string { return "http_request" }

func (t *RequestTool) Description() string {
	return "Make an HTTP request to an external API. Supports GET, POST, PUT, PATCH, DELETE methods."
}

func (t *RequestTool) ToolInfo() *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: t.Name(),
		Desc: t.Description(),
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"url": {
				Type:     schema.String,
				Desc:     "The target URL (must be http or https)",
				Required: true,
			},
			"method": {
				Type:     schema.String,
				Desc:     "HTTP method: GET, POST, PUT, PATCH, DELETE",
				Required: true,
			},
			"headers": {
				Type: schema.Object,
				Desc: "Custom request headers as key-value pairs",
			},
			"body": {
				Type: schema.String,
				Desc: "Request body (typically a JSON string)",
			},
			"timeout": {
				Type: schema.Integer,
				Desc: "Timeout in seconds (default 30, max 120)",
			},
		}),
	}
}

type requestResult struct {
	Status    int               `json:"status"`
	Headers   map[string]string `json:"headers"`
	Body      string            `json:"body"`
	Length    int               `json:"length"`
	Truncated bool              `json:"truncated"`
}

func (t *RequestTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	// Parse and validate URL.
	rawURL, _ := args["url"].(string)
	if rawURL == "" {
		return nil, fmt.Errorf("url is required")
	}
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

	// Parse and validate method.
	method := strings.ToUpper(strings.TrimSpace(args["method"].(string)))
	if !allowedMethods[method] {
		return nil, fmt.Errorf("unsupported method %q; allowed: GET, POST, PUT, PATCH, DELETE", method)
	}

	// Parse optional body.
	body, _ := args["body"].(string)

	// Parse optional timeout.
	timeout := defaultTimeout
	if v, ok := args["timeout"].(float64); ok && v > 0 {
		timeout = time.Duration(v) * time.Second
		if timeout > maxTimeout {
			timeout = maxTimeout
		}
	}

	// Build the request.
	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, rawURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	// Apply custom headers.
	if hdrs, ok := args["headers"].(map[string]interface{}); ok {
		for k, v := range hdrs {
			if s, ok := v.(string); ok {
				req.Header.Set(k, s)
			}
		}
	}

	// Default Content-Type for requests with a body.
	if body != "" && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	// Build client with SSRF-safe redirect policy.
	client := &http.Client{
		CheckRedirect: func(r *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("too many redirects (max %d)", maxRedirects)
			}
			if isPrivateHost(r.URL.Hostname()) {
				return fmt.Errorf("redirect to private address blocked")
			}
			return nil
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read body with size limit.
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyMiB*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	// Collect response headers (first value only).
	respHeaders := make(map[string]string, len(resp.Header))
	for k := range resp.Header {
		respHeaders[k] = resp.Header.Get(k)
	}

	content := string(respBody)
	truncated := false
	if len(content) > maxResponseChar {
		content = content[:maxResponseChar]
		truncated = true
	}

	result := requestResult{
		Status:    resp.StatusCode,
		Headers:   respHeaders,
		Body:      content,
		Length:    len(content),
		Truncated: truncated,
	}

	logs.CtxInfo(ctx, "[tool:http_request] %s %s → %d (%d chars, truncated=%v)",
		method, rawURL, resp.StatusCode, result.Length, truncated)

	out, _ := sonic.MarshalString(result)
	return out, nil
}
