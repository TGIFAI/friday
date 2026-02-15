package webx

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/cloudwego/eino/schema"

	"github.com/tgifai/friday/internal/pkg/logs"
)

const defaultSearchCount = 5

// SearchProvider is the interface that search backends must implement.
type SearchProvider interface {
	Search(ctx context.Context, query string, count int) ([]SearchResult, error)
}

// SearchResult is a single search hit.
type SearchResult struct {
	Title   string
	URL     string
	Snippet string
}

// SearchTool lets the agent search the web.
type SearchTool struct {
	once     sync.Once
	provider SearchProvider
	initErr  error
}

// NewSearchTool creates a SearchTool backed by the Brave Search API.
// The API key is read from BRAVE_API_KEY at execution time.
func NewSearchTool() *SearchTool {
	return &SearchTool{}
}

func (t *SearchTool) Name() string { return "web_search" }

func (t *SearchTool) Description() string {
	return "Search the web. Returns titles, URLs, and snippets for the top results."
}

func (t *SearchTool) ToolInfo() *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: t.Name(),
		Desc: t.Description(),
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {
				Type:     schema.String,
				Desc:     "The search query",
				Required: true,
			},
			"count": {
				Type: schema.Integer,
				Desc: "Number of results to return (1-10, default 5)",
			},
		}),
	}
}

func (t *SearchTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	t.once.Do(func() {
		key := os.Getenv("BRAVE_API_KEY")
		if key == "" {
			t.initErr = fmt.Errorf("BRAVE_API_KEY environment variable is not set; web search is unavailable")
			return
		}
		t.provider = newBraveProvider(key)
	})
	if t.initErr != nil {
		return nil, t.initErr
	}

	query, _ := args["query"].(string)
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}

	count := defaultSearchCount
	if v, ok := args["count"].(float64); ok && v > 0 {
		count = int(v)
	}
	if count < 1 {
		count = 1
	}
	if count > 10 {
		count = 10
	}

	results, err := t.provider.Search(ctx, query, count)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	if len(results) == 0 {
		logs.CtxInfo(ctx, "[tool:web_search] no results for: %s", query)
		return fmt.Sprintf("No results found for: %s", query), nil
	}

	logs.CtxInfo(ctx, "[tool:web_search] %q → %d results", query, len(results))

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Results for: %s\n\n", query))
	for i, r := range results {
		sb.WriteString(fmt.Sprintf("%d. %s\n   %s\n", i+1, r.Title, r.URL))
		if r.Snippet != "" {
			sb.WriteString(fmt.Sprintf("   %s\n", r.Snippet))
		}
		sb.WriteByte('\n')
	}
	return sb.String(), nil
}
