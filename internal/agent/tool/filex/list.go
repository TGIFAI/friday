package filex

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cloudwego/eino/schema"
	"github.com/tgifai/friday/internal/pkg/logs"
)

type ListTool struct {
	guard *fsGuard
}

func NewListTool(workspace string, allowedPaths []string) *ListTool {
	return &ListTool{guard: newFSGuard(workspace, allowedPaths)}
}

func (t *ListTool) Name() string { return "list" }

func (t *ListTool) Description() string { return "List files in an allowed directory" }

func (t *ListTool) ToolInfo() *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: t.Name(),
		Desc: t.Description(),
		Extra: map[string]interface{}{
			"path": "string (required) - directory path",
		},
	}
}

func (t *ListTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	path, _ := args["path"].(string)
	absPath, err := t.guard.resolvePath(path)
	if err != nil {
		return nil, err
	}
	if err := t.guard.checkPathAllowed(absPath); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to list directory: %w", err)
	}
	files := make([]map[string]interface{}, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		entryType := "file"
		if entry.IsDir() {
			entryType = "directory"
		}
		files = append(files, map[string]interface{}{
			"name": entry.Name(),
			"path": filepath.Join(path, entry.Name()),
			"type": entryType,
			"size": info.Size(),
			"mode": info.Mode().String(),
		})
	}
	logs.CtxInfo(ctx, "[tool:list] %s (%d entries)", path, len(files))
	return map[string]interface{}{
		"success": true,
		"path":    path,
		"files":   files,
		"count":   len(files),
	}, nil
}
