package filex

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/bytedance/gg/gconv"
	"github.com/cloudwego/eino/schema"

	"github.com/tgifai/friday/internal/pkg/logs"
)

type EditTool struct {
	guard *fsGuard
}

func NewEditTool(workspace string, allowedPaths []string) *EditTool {
	return &EditTool{guard: newFSGuard(workspace, allowedPaths)}
}

func (t *EditTool) Name() string { return "edit" }

func (t *EditTool) Description() string {
	return "Edit file by text replacement or line range replacement"
}

func (t *EditTool) ToolInfo() *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: t.Name(),
		Desc: t.Description(),
		Extra: map[string]interface{}{
			"path":        "string (required) - file path",
			"old_text":    "string (optional) - text to replace",
			"new_text":    "string (required) - replacement text",
			"replace_all": "bool (optional, default true) - replace all matches",
			"start_line":  "number (optional) - 1-based start line",
			"end_line":    "number (optional) - 1-based end line",
		},
	}
}

func (t *EditTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	path, _ := args["path"].(string)
	absPath, err := t.guard.resolvePath(path)
	if err != nil {
		return nil, err
	}
	if err := t.guard.checkPathAllowed(absPath); err != nil {
		return nil, err
	}

	raw, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}
	original := string(raw)
	updated, edits, err := applyEdit(original, args)
	if err != nil {
		return nil, err
	}
	if updated == original {
		return nil, fmt.Errorf("edit made no changes")
	}
	if err := os.WriteFile(absPath, []byte(updated), 0o644); err != nil {
		return nil, fmt.Errorf("failed to write file: %w", err)
	}
	logs.CtxInfo(ctx, "[tool:edit] %s (%d changes)", path, edits)
	return map[string]interface{}{
		"success": true,
		"path":    path,
		"changes": edits,
		"size":    len(updated),
	}, nil
}

func applyEdit(content string, args map[string]interface{}) (string, int, error) {
	newText, ok := args["new_text"].(string)
	if !ok {
		return "", 0, fmt.Errorf("new_text is required")
	}

	oldText, hasOldText := args["old_text"].(string)
	startLine := gconv.To[int](args["start_line"])
	endLine := gconv.To[int](args["end_line"])

	if hasOldText {
		if oldText == "" {
			return "", 0, fmt.Errorf("old_text must not be empty when provided")
		}
		replaceAll := true
		if raw, ok := args["replace_all"]; ok {
			replaceAll = gconv.To[bool](raw)
		}
		count := strings.Count(content, oldText)
		if count == 0 {
			return "", 0, fmt.Errorf("old_text not found in file")
		}
		if replaceAll {
			return strings.ReplaceAll(content, oldText, newText), count, nil
		}
		return strings.Replace(content, oldText, newText, 1), 1, nil
	}

	if startLine <= 0 || endLine <= 0 {
		return "", 0, fmt.Errorf("either old_text or start_line/end_line is required")
	}
	if endLine < startLine {
		return "", 0, fmt.Errorf("end_line must be >= start_line")
	}

	lines := strings.Split(content, "\n")
	if startLine > len(lines) || endLine > len(lines) {
		return "", 0, fmt.Errorf("line range out of bounds")
	}

	before := append([]string{}, lines[:startLine-1]...)
	after := append([]string{}, lines[endLine:]...)
	replacement := strings.Split(newText, "\n")
	merged := append(before, replacement...)
	merged = append(merged, after...)
	return strings.Join(merged, "\n"), 1, nil
}
