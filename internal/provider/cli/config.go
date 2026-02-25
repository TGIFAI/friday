package cli

import (
	"errors"
	"fmt"
	"time"

	"github.com/bytedance/gg/gconv"
)

type Config struct {
	ID           string
	Backend      string // "claude-code" or "codex"
	DefaultModel string
	Timeout      time.Duration
	WorkDir      string
}

func (c *Config) Validate() error {
	if c.ID == "" {
		return errors.New("provider ID cannot be empty")
	}
	switch c.Backend {
	case "claude-code", "codex":
		// ok
	case "":
		return errors.New("backend is required (claude-code or codex)")
	default:
		return fmt.Errorf("unsupported backend: %s (supported: claude-code, codex)", c.Backend)
	}
	return nil
}

func ParseConfig(id string, configMap map[string]interface{}) (*Config, error) {
	cfg := &Config{ID: id}

	cfg.Backend = gconv.To[string](configMap["backend"])

	if defaultModel := gconv.To[string](configMap["default_model"]); defaultModel != "" {
		cfg.DefaultModel = defaultModel
	}

	if timeout := gconv.To[int](configMap["timeout"]); timeout > 0 {
		cfg.Timeout = time.Duration(timeout) * time.Second
	} else {
		cfg.Timeout = 300 * time.Second
	}

	if workDir := gconv.To[string](configMap["work_dir"]); workDir != "" {
		cfg.WorkDir = workDir
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid cli config: %w", err)
	}

	return cfg, nil
}
