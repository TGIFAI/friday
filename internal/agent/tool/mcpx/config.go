package mcpx

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/bytedance/sonic"
)

// ServerConfig describes how to connect to a single MCP server.
type ServerConfig struct {
	Transport string            `json:"transport"` // "stdio" or "http"
	Command   string            `json:"command"`   // stdio only
	Args      []string          `json:"args"`      // stdio only
	Env       map[string]string `json:"env"`       // stdio only
	URL       string            `json:"url"`       // http only
}

// Config holds all MCP server definitions loaded from <AGENT_WORKSPACE>/mcp.json.
type Config struct {
	MCPServers map[string]ServerConfig `json:"mcpServers"`
}

// loadConfig reads MCP server configuration from <AGENT_WORKSPACE>/mcp.json.
// Returns nil (not error) if the file doesn't exist.
func loadConfig(workspace string) (*Config, error) {
	p := filepath.Join(workspace, "mcp.json")

	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read mcp config: %w", err)
	}

	var cfg Config
	if err := sonic.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse mcp config: %w", err)
	}

	for name, sc := range cfg.MCPServers {
		if err := validateServerConfig(name, sc); err != nil {
			return nil, err
		}
	}

	return &cfg, nil
}

func validateServerConfig(name string, sc ServerConfig) error {
	switch sc.Transport {
	case "stdio":
		if sc.Command == "" {
			return fmt.Errorf("mcp server %q: stdio transport requires command", name)
		}
	case "http":
		if sc.URL == "" {
			return fmt.Errorf("mcp server %q: http transport requires url", name)
		}
	default:
		return fmt.Errorf("mcp server %q: unsupported transport %q (use \"stdio\" or \"http\")", name, sc.Transport)
	}
	return nil
}
