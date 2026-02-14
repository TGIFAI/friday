package filex

import (
	"context"
	"fmt"
	"os"

	"github.com/cloudwego/eino/schema"
	"github.com/tgifai/friday/internal/pkg/logs"
)

type DeleteTool struct {
	guard *fsGuard
}

func NewDeleteTool(workspace string, allowedPaths []string) *DeleteTool {
	return &DeleteTool{guard: newFSGuard(workspace, allowedPaths)}
}

func (t *DeleteTool) Name() string { return "delete" }

func (t *DeleteTool) Description() string { return "Delete a file in allowed paths" }

func (t *DeleteTool) ToolInfo() *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: t.Name(),
		Desc: t.Description(),
		Extra: map[string]interface{}{
			"path": "string (required) - file path",
		},
	}
}

func (t *DeleteTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	path, _ := args["path"].(string)
	absPath, err := t.guard.resolvePath(path)
	if err != nil {
		return nil, err
	}
	if err := t.guard.checkPathAllowed(absPath); err != nil {
		return nil, err
	}
	if err := os.Remove(absPath); err != nil {
		return nil, fmt.Errorf("failed to delete file: %w", err)
	}
	logs.CtxInfo(ctx, "[tool:delete] %s", path)
	return map[string]interface{}{
		"success": true,
		"path":    path,
	}, nil
}
