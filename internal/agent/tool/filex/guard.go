package filex

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type fsGuard struct {
	workspace    string
	allowedPaths []string
}

func newFSGuard(workspace string, allowedPaths []string) *fsGuard {
	return &fsGuard{
		workspace:    workspace,
		allowedPaths: allowedPaths,
	}
}

func (g *fsGuard) resolvePath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}
	if g.workspace != "" {
		return filepath.Clean(filepath.Join(g.workspace, path)), nil
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("failed to resolve path: %w", err)
	}
	return absPath, nil
}

func (g *fsGuard) checkPathAllowed(path string) error {
	if len(g.allowedPaths) == 0 {
		return nil
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}
	for _, allowed := range g.allowedPaths {
		allowedPath := strings.TrimSpace(allowed)
		if allowedPath == "" {
			continue
		}
		allowedAbs, err := filepath.Abs(allowedPath)
		if err != nil {
			continue
		}
		ok, err := isPathWithin(absPath, allowedAbs)
		if err == nil && ok {
			return nil
		}
	}
	return fmt.Errorf("path not allowed: %s", path)
}

func isPathWithin(path string, root string) (bool, error) {
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return false, err
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false, err
	}
	pathClean := filepath.Clean(pathAbs)
	rootClean := filepath.Clean(rootAbs)
	rel, err := filepath.Rel(rootClean, pathClean)
	if err != nil {
		return false, err
	}
	if rel == "." {
		return true, nil
	}
	if strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || rel == ".." {
		return false, nil
	}
	return true, nil
}
