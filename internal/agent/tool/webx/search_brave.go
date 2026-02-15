package webx

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/bytedance/sonic"
)

const (
	braveEndpoint = "https://api.search.brave.com/res/v1/web/search"
	braveTimeout  = 10 * time.Second
)

type braveProvider struct {
	apiKey string
	client *http.Client
}

func newBraveProvider(apiKey string) *braveProvider {
	return &braveProvider{
		apiKey: apiKey,
		client: &http.Client{
			Timeout:   braveTimeout,
			Transport: newCompressedTransport(nil),
		},
	}
}

func (p *braveProvider) Search(ctx context.Context, query string, count int) ([]SearchResult, error) {
	params := url.Values{}
	params.Set("q", query)
	params.Set("count", fmt.Sprintf("%d", count))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, braveEndpoint+"?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("brave search request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("brave search HTTP %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var parsed braveResponse
	if err := sonic.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	results := make([]SearchResult, 0, len(parsed.Web.Results))
	for _, r := range parsed.Web.Results {
		results = append(results, SearchResult{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: r.Description,
		})
	}
	return results, nil
}

// braveResponse mirrors the relevant fields from the Brave Search API response.
type braveResponse struct {
	Web struct {
		Results []struct {
			Title       string `json:"title"`
			URL         string `json:"url"`
			Description string `json:"description"`
		} `json:"results"`
	} `json:"web"`
}
