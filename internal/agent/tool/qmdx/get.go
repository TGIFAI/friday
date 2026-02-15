package qmdx

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"

	"github.com/tgifai/friday/internal/pkg/logs"
)

const defaultGetTimeout = 15 * time.Second

// GetTool retrieves a specific document from the qmd knowledge base by path or ID.
type GetTool struct{}

func NewGetTool() *GetTool { return &GetTool{} }

func (t *GetTool) Name() string { return "knowledge_get" }

func (t *GetTool) Description() string {
	return "Retrieve a specific document from the local knowledge base by file path or document ID (e.g. #abc123). Use knowledge_search first to find relevant documents, then use this tool only when you need the full content."
}

func (t *GetTool) ToolInfo() *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: t.Name(),
		Desc: t.Description(),
		Extra: map[string]interface{}{
			"path": "string (required) - file path or document ID (#abc123)",
		},
	}
}

func (t *GetTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	path, _ := args["path"].(string)
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}

	cmdCtx, cancel := context.WithTimeout(ctx, defaultGetTimeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "qmd", "get", path, "--format", "markdown")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("qmd get failed: %v (stderr: %s)", err, stderr.String())
	}

	content := stdout.String()
	logs.CtxInfo(ctx, "[tool:knowledge_get] path=%s (%d bytes)", path, len(content))

	return map[string]interface{}{
		"success": true,
		"path":    path,
		"content": content,
		"size":    len(content),
	}, nil
}
