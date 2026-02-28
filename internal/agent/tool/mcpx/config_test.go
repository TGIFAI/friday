package mcpx

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_FileNotExist(t *testing.T) {
	cfg, err := LoadConfig(t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg != nil {
		t.Fatal("expected nil config for missing file")
	}
}

func TestLoadConfig_ValidStdio(t *testing.T) {
	dir := t.TempDir()
	fridayDir := filepath.Join(dir, ".friday")
	if err := os.MkdirAll(fridayDir, 0o755); err != nil {
		t.Fatal(err)
	}

	data := `{
		"mcpServers": {
			"context7": {
				"transport": "stdio",
				"command": "npx",
				"args": ["-y", "@upstash/context7-mcp"],
				"env": {"NODE_ENV": "production"}
			}
		}
	}`
	if err := os.WriteFile(filepath.Join(fridayDir, "mcp.json"), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if len(cfg.MCPServers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(cfg.MCPServers))
	}

	sc, ok := cfg.MCPServers["context7"]
	if !ok {
		t.Fatal("expected 'context7' server")
	}
	if sc.Transport != "stdio" {
		t.Errorf("transport = %q, want %q", sc.Transport, "stdio")
	}
	if sc.Command != "npx" {
		t.Errorf("command = %q, want %q", sc.Command, "npx")
	}
	if len(sc.Args) != 2 || sc.Args[0] != "-y" {
		t.Errorf("args = %v, want [-y @upstash/context7-mcp]", sc.Args)
	}
	if sc.Env["NODE_ENV"] != "production" {
		t.Errorf("env[NODE_ENV] = %q, want %q", sc.Env["NODE_ENV"], "production")
	}
}

func TestLoadConfig_ValidHTTP(t *testing.T) {
	dir := t.TempDir()
	fridayDir := filepath.Join(dir, ".friday")
	if err := os.MkdirAll(fridayDir, 0o755); err != nil {
		t.Fatal(err)
	}

	data := `{
		"mcpServers": {
			"remote": {
				"transport": "http",
				"url": "http://localhost:8080/mcp"
			}
		}
	}`
	if err := os.WriteFile(filepath.Join(fridayDir, "mcp.json"), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sc := cfg.MCPServers["remote"]
	if sc.Transport != "http" {
		t.Errorf("transport = %q, want %q", sc.Transport, "http")
	}
	if sc.URL != "http://localhost:8080/mcp" {
		t.Errorf("url = %q, want %q", sc.URL, "http://localhost:8080/mcp")
	}
}

func TestLoadConfig_InvalidTransport(t *testing.T) {
	dir := t.TempDir()
	fridayDir := filepath.Join(dir, ".friday")
	if err := os.MkdirAll(fridayDir, 0o755); err != nil {
		t.Fatal(err)
	}

	data := `{"mcpServers": {"bad": {"transport": "grpc"}}}`
	if err := os.WriteFile(filepath.Join(fridayDir, "mcp.json"), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadConfig(dir)
	if err == nil {
		t.Fatal("expected error for invalid transport")
	}
}

func TestLoadConfig_StdioMissingCommand(t *testing.T) {
	dir := t.TempDir()
	fridayDir := filepath.Join(dir, ".friday")
	if err := os.MkdirAll(fridayDir, 0o755); err != nil {
		t.Fatal(err)
	}

	data := `{"mcpServers": {"bad": {"transport": "stdio"}}}`
	if err := os.WriteFile(filepath.Join(fridayDir, "mcp.json"), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadConfig(dir)
	if err == nil {
		t.Fatal("expected error for stdio without command")
	}
}

func TestLoadConfig_HTTPMissingURL(t *testing.T) {
	dir := t.TempDir()
	fridayDir := filepath.Join(dir, ".friday")
	if err := os.MkdirAll(fridayDir, 0o755); err != nil {
		t.Fatal(err)
	}

	data := `{"mcpServers": {"bad": {"transport": "http"}}}`
	if err := os.WriteFile(filepath.Join(fridayDir, "mcp.json"), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadConfig(dir)
	if err == nil {
		t.Fatal("expected error for http without url")
	}
}

func TestLoadConfig_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	fridayDir := filepath.Join(dir, ".friday")
	if err := os.MkdirAll(fridayDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(fridayDir, "mcp.json"), []byte("{invalid"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadConfig(dir)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
