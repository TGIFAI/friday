package filex

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cloudwego/eino/schema"
	"github.com/tgifai/friday/internal/pkg/logs"
)

type FileTool struct {
	allowedPaths []string
	workspace    string
}

func NewFileTool(workspace string, allowedPaths []string) *FileTool {
	return &FileTool{
		allowedPaths: allowedPaths,
		workspace:    workspace,
	}
}

func (t *FileTool) Name() string {
	return "file"
}

func (t *FileTool) Description() string {
	return "File operations: read_file, write_file, list_dir, delete_file"
}

func (t *FileTool) ToolInfo() *schema.ToolInfo {

	return &schema.ToolInfo{
		Name: "file",
		Desc: "File operations: read_file(path), write_file(path, content), list_dir(path), delete_file(path)",

		ParamsOneOf: nil,
		Extra: map[string]interface{}{
			"operations": []string{"read_file", "write_file", "list_dir", "delete_file"},
			"read_file": map[string]interface{}{
				"path": "string (required) - File path to read",
			},
			"write_file": map[string]interface{}{
				"path":    "string (required) - File path to write",
				"content": "string (required) - Content to write",
			},
			"list_dir": map[string]interface{}{
				"path": "string (required) - Directory path to list",
			},
			"delete_file": map[string]interface{}{
				"path": "string (required) - File path to delete",
			},
		},
	}
}

func (t *FileTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {

	operation, ok := args["operation"].(string)
	if !ok {

		if _, hasPath := args["path"]; hasPath {
			if _, hasContent := args["content"]; hasContent {
				operation = "write_file"
			} else {

				path, _ := args["path"].(string)
				if path != "" {
					absPath, err := t.resolvePath(path)
					if err != nil {
						return nil, err
					}
					info, err := os.Stat(absPath)
					if err == nil {
						if info.IsDir() {
							operation = "list_dir"
						} else {
							operation = "read_file"
						}
					} else {

						operation = "read_file"
					}
				}
			}
		}
	}

	if operation == "" {
		return nil, fmt.Errorf("operation not specified, use one of: read_file, write_file, list_dir, delete_file")
	}

	switch operation {
	case "read_file":
		return t.readFile(ctx, args)
	case "write_file":
		return t.writeFile(ctx, args)
	case "list_dir":
		return t.listDir(ctx, args)
	case "delete_file":
		return t.deleteFile(ctx, args)
	default:
		return nil, fmt.Errorf("unknown operation: %s", operation)
	}
}

func (t *FileTool) readFile(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	path, ok := args["path"].(string)
	if !ok || path == "" {
		return nil, fmt.Errorf("path is required")
	}

	absPath, err := t.resolvePath(path)
	if err != nil {
		return nil, err
	}

	if err := t.checkPathAllowed(absPath); err != nil {
		return nil, err
	}

	content, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	logs.CtxInfo(ctx, "[tool:file] read_file: %s (%d bytes)", path, len(content))
	return map[string]interface{}{
		"success": true,
		"path":    path,
		"content": string(content),
		"size":    len(content),
	}, nil
}

func (t *FileTool) writeFile(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	path, ok := args["path"].(string)
	if !ok || path == "" {
		return nil, fmt.Errorf("path is required")
	}

	content, ok := args["content"].(string)
	if !ok {
		return nil, fmt.Errorf("content is required")
	}

	absPath, err := t.resolvePath(path)
	if err != nil {
		return nil, err
	}

	if err := t.checkPathAllowed(absPath); err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create parent directory: %w", err)
	}

	if err := os.WriteFile(absPath, []byte(content), 0644); err != nil {
		return nil, fmt.Errorf("failed to write file: %w", err)
	}

	logs.CtxInfo(ctx, "[tool:file] write_file: %s (%d bytes)", path, len(content))
	return map[string]interface{}{
		"success": true,
		"path":    path,
		"size":    len(content),
	}, nil
}

func (t *FileTool) listDir(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	path, ok := args["path"].(string)
	if !ok || path == "" {
		return nil, fmt.Errorf("path is required")
	}

	absPath, err := t.resolvePath(path)
	if err != nil {
		return nil, err
	}

	if err := t.checkPathAllowed(absPath); err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to list directory: %w", err)
	}

	var files []map[string]interface{}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}

		files = append(files, map[string]interface{}{
			"name": entry.Name(),
			"path": filepath.Join(path, entry.Name()),
			"type": func() string {
				if entry.IsDir() {
					return "directory"
				} else {
					return "file"
				}
			}(),
			"size":  info.Size(),
			"mode":  info.Mode().String(),
			"mtime": info.ModTime().Unix(),
		})
	}

	logs.CtxInfo(ctx, "[tool:file] list_dir: %s (%d entries)", path, len(files))
	return map[string]interface{}{
		"success": true,
		"path":    path,
		"files":   files,
		"count":   len(files),
	}, nil
}

func (t *FileTool) deleteFile(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	path, ok := args["path"].(string)
	if !ok || path == "" {
		return nil, fmt.Errorf("path is required")
	}

	absPath, err := t.resolvePath(path)
	if err != nil {
		return nil, err
	}

	if err := t.checkPathAllowed(absPath); err != nil {
		return nil, err
	}

	if err := os.Remove(absPath); err != nil {
		return nil, fmt.Errorf("failed to delete file: %w", err)
	}

	logs.CtxInfo(ctx, "[tool:file] delete_file: %s", path)
	return map[string]interface{}{
		"success": true,
		"path":    path,
	}, nil
}

func (t *FileTool) resolvePath(path string) (string, error) {

	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}

	if t.workspace != "" {
		return filepath.Clean(filepath.Join(t.workspace, path)), nil
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("failed to resolve path: %w", err)
	}

	return absPath, nil
}

func (t *FileTool) checkPathAllowed(path string) error {

	if len(t.allowedPaths) == 0 {
		return nil
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}

	for _, allowed := range t.allowedPaths {
		allowedAbs, err := filepath.Abs(allowed)
		if err != nil {
			continue
		}

		ok, relErr := isPathWithin(absPath, allowedAbs)
		if relErr == nil && ok {
			return nil
		}
	}

	return fmt.Errorf("path not allowed: %s", path)
}
