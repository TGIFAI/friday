package filex

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cloudwego/eino/schema"
	"github.com/tgifai/friday/internal/pkg/logs"
)

type WriteTool struct {
	guard *fsGuard
}

func NewWriteTool(workspace string, allowedPaths []string) *WriteTool {
	return &WriteTool{guard: newFSGuard(workspace, allowedPaths)}
}

func (t *WriteTool) Name() string { return "write" }

func (t *WriteTool) Description() string { return "Write content to a file in allowed paths" }

func (t *WriteTool) ToolInfo() *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: t.Name(),
		Desc: t.Description(),
		Extra: map[string]interface{}{
			"path":    "string (required) - file path",
			"content": "string (required) - full file content",
		},
	}
}

func (t *WriteTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	path, _ := args["path"].(string)
	content, ok := args["content"].(string)
	if !ok {
		return nil, fmt.Errorf("content is required")
	}
	absPath, err := t.guard.resolvePath(path)
	if err != nil {
		return nil, err
	}
	if err := t.guard.checkPathAllowed(absPath); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return nil, fmt.Errorf("failed to create parent directory: %w", err)
	}
	if err := os.WriteFile(absPath, []byte(content), 0o644); err != nil {
		return nil, fmt.Errorf("failed to write file: %w", err)
	}
	logs.CtxInfo(ctx, "[tool:write] %s (%d bytes)", path, len(content))
	return map[string]interface{}{
		"success": true,
		"path":    path,
		"size":    len(content),
	}, nil
}
