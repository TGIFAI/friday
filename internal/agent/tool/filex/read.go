package filex

import (
	"context"
	"fmt"
	"os"

	"github.com/cloudwego/eino/schema"
	"github.com/tgifai/friday/internal/pkg/logs"
)

type ReadTool struct {
	guard *fsGuard
}

func NewReadTool(workspace string, allowedPaths []string) *ReadTool {
	return &ReadTool{guard: newFSGuard(workspace, allowedPaths)}
}

func (t *ReadTool) Name() string { return "read" }

func (t *ReadTool) Description() string { return "Read file content from an allowed path" }

func (t *ReadTool) ToolInfo() *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: t.Name(),
		Desc: t.Description(),
		Extra: map[string]interface{}{
			"path": "string (required) - file path",
		},
	}
}

func (t *ReadTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	path, _ := args["path"].(string)
	absPath, err := t.guard.resolvePath(path)
	if err != nil {
		return nil, err
	}
	if err := t.guard.checkPathAllowed(absPath); err != nil {
		return nil, err
	}
	content, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}
	logs.CtxInfo(ctx, "[tool:read] %s (%d bytes)", path, len(content))
	return map[string]interface{}{
		"success": true,
		"path":    path,
		"content": string(content),
		"size":    len(content),
	}, nil
}
