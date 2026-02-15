package webx

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/bytedance/sonic"
)

const (
	cfTimeout = 30 * time.Second
)

// cloudflareRenderer uses Cloudflare's Browser Rendering /markdown endpoint
// to convert a URL into clean markdown via headless browser rendering.
// Requires CLOUDFLARE_API_TOKEN and CLOUDFLARE_ACCOUNT_ID env vars.
type cloudflareRenderer struct {
	accountID string
	apiToken  string
	client    *http.Client
}

var (
	cfOnce     sync.Once
	cfRenderer *cloudflareRenderer
)

// getCloudflareRenderer returns the singleton renderer if env vars are set, or nil.
func getCloudflareRenderer() *cloudflareRenderer {
	cfOnce.Do(func() {
		accountID := os.Getenv("CLOUDFLARE_ACCOUNT_ID")
		apiToken := os.Getenv("CLOUDFLARE_API_TOKEN")
		if accountID == "" || apiToken == "" {
			return
		}
		cfRenderer = &cloudflareRenderer{
			accountID: accountID,
			apiToken:  apiToken,
			client:    &http.Client{Timeout: cfTimeout},
		}
	})
	return cfRenderer
}

type cfMarkdownRequest struct {
	URL string `json:"url"`
}

type cfMarkdownResponse struct {
	Success bool `json:"success"`
	Result  struct {
		Markdown string `json:"markdown"`
		Title    string `json:"title"`
	} `json:"result"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// RenderMarkdown calls the Cloudflare Browser Rendering /markdown endpoint.
func (r *cloudflareRenderer) RenderMarkdown(ctx context.Context, targetURL string) (title, markdown string, err error) {
	endpoint := fmt.Sprintf(
		"https://api.cloudflare.com/client/v4/accounts/%s/browser-rendering/markdown",
		r.accountID,
	)

	payload, _ := sonic.Marshal(cfMarkdownRequest{URL: targetURL})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", "", fmt.Errorf("create cloudflare request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+r.apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("cloudflare request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return "", "", fmt.Errorf("read cloudflare response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("cloudflare HTTP %d: %s", resp.StatusCode, string(body[:min(512, len(body))]))
	}

	var cfResp cfMarkdownResponse
	if err := sonic.Unmarshal(body, &cfResp); err != nil {
		return "", "", fmt.Errorf("parse cloudflare response: %w", err)
	}

	if !cfResp.Success {
		msg := "unknown error"
		if len(cfResp.Errors) > 0 {
			msg = cfResp.Errors[0].Message
		}
		return "", "", fmt.Errorf("cloudflare API error: %s", msg)
	}

	return cfResp.Result.Title, cfResp.Result.Markdown, nil
}
