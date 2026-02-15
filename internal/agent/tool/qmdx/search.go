package qmdx

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/bytedance/gg/gconv"
	"github.com/bytedance/sonic"
	"github.com/cloudwego/eino/schema"

	"github.com/tgifai/friday/internal/pkg/logs"
)

const (
	defaultSearchLimit   = 5
	defaultSearchTimeout = 30 * time.Second
)

// SearchTool exposes qmd's hybrid search as an agent tool.
type SearchTool struct{}

func NewSearchTool() *SearchTool { return &SearchTool{} }

func (t *SearchTool) Name() string { return "knowledge_search" }

func (t *SearchTool) Description() string {
	return "Search the local knowledge base (markdown docs, notes, meeting transcripts) using hybrid BM25 + vector semantic search. Returns relevant snippets instead of full documents to save tokens."
}

func (t *SearchTool) ToolInfo() *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: t.Name(),
		Desc: t.Description(),
		Extra: map[string]interface{}{
			"query":      "string (required) - search keywords or natural-language question",
			"collection": "string (optional) - restrict search to a named collection",
			"mode":       "string (optional) - 'query' (default, hybrid+rerank), 'search' (BM25 only), 'vsearch' (vector only)",
			"limit":      "number (optional) - max results, default 5",
		},
	}
}

func (t *SearchTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	query, _ := args["query"].(string)
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}

	mode, _ := args["mode"].(string)
	switch mode {
	case "search", "vsearch", "query":
		// valid
	case "":
		mode = "query"
	default:
		return nil, fmt.Errorf("mode must be one of: query, search, vsearch")
	}

	limit := gconv.To[int](args["limit"])
	if limit <= 0 {
		limit = defaultSearchLimit
	}

	cmdArgs := []string{mode, query, "--format", "json", "--limit", fmt.Sprintf("%d", limit)}

	if col, _ := args["collection"].(string); col != "" {
		cmdArgs = append(cmdArgs, "--collection", col)
	}

	cmdCtx, cancel := context.WithTimeout(ctx, defaultSearchTimeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "qmd", cmdArgs...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("qmd %s failed: %v (stderr: %s)", mode, err, stderr.String())
	}

	var results []map[string]interface{}
	if err := sonic.Unmarshal(stdout.Bytes(), &results); err != nil {
		// If JSON parsing fails, return raw text so the LLM can still use it.
		logs.CtxWarn(ctx, "[tool:knowledge_search] json parse failed, returning raw output")
		return map[string]interface{}{
			"success": true,
			"query":   query,
			"mode":    mode,
			"raw":     stdout.String(),
		}, nil
	}

	logs.CtxInfo(ctx, "[tool:knowledge_search] mode=%s query=%q results=%d", mode, query, len(results))

	return map[string]interface{}{
		"success": true,
		"query":   query,
		"mode":    mode,
		"count":   len(results),
		"results": results,
	}, nil
}
