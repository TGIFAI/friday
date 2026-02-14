package config

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/bytedance/sonic"

	"github.com/tgifai/friday/internal/consts"
)

type (
	Config struct {
		Gateway   GatewayConfig             `yaml:"gateway"`
		Logging   LoggingConfig             `yaml:"logging"`
		Cronjob   CronjobConfig             `yaml:"cronjob"`
		Agents    map[string]AgentConfig    `yaml:"agents"`
		Channels  map[string]ChannelConfig  `yaml:"channels"`
		Providers map[string]ProviderConfig `yaml:"providers"`
	}

	GatewayConfig struct {
		Bind                  string `yaml:"bind"`
		MaxConcurrentSessions int    `yaml:"max_concurrent_sessions"`
		RequestTimeout        int    `yaml:"request_timeout"`
		AutoUpdate            bool   `yaml:"auto_update"`
	}

	LoggingConfig struct {
		Level      string `yaml:"level"`  // debug, info, warn, error
		Format     string `yaml:"format"` // json, text
		Output     string `yaml:"output"` // stdout, file, both
		File       string `yaml:"file"`
		MaxSize    int    `yaml:"max_size"` // MB
		MaxBackups int    `yaml:"max_backups"`
		MaxAge     int    `yaml:"max_age"` // days
	}

	CronjobConfig struct {
		Enabled           *bool  `yaml:"enabled"`
		Store             string `yaml:"store"`
		MaxConcurrentRuns int    `yaml:"max_concurrent_runs"`
		JobTimeoutSec     int    `yaml:"job_timeout_sec"`
		SessionRetention  string `yaml:"session_retention"`
	}

	AgentConfig struct {
		ID        string             `yaml:"-"`
		Name      string             `yaml:"name"`
		Workspace string             `yaml:"workspace"`
		Channels  []string           `yaml:"channels"`
		Skills    []string           `yaml:"skills"`
		Models    ModelsConfig       `yaml:"models"`
		Config    AgentRuntimeConfig `yaml:"config"`
	}

	ModelsConfig struct {
		Primary  string   `yaml:"primary"`
		Fallback []string `yaml:"fallback"`
	}

	AgentRuntimeConfig struct {
		MaxIterations int     `yaml:"max_iterations"`
		MaxTokens     int     `yaml:"max_tokens"`
		Temperature   float64 `yaml:"temperature"`
	}

	ChannelConfig struct {
		ID       string                      `yaml:"-"`
		Type     string                      `yaml:"type"` // telegram, lark, discord, http
		Enabled  bool                        `yaml:"enabled"`
		ACL      map[string]ChannelACLConfig `yaml:"acl,omitempty"` // key: chatType:chatId
		Security ChannelSecurityConfig       `yaml:"security,omitempty"`
		Config   map[string]interface{}      `yaml:"config"`
	}

	ChannelACLConfig struct {
		Allow []string `yaml:"allow"`
		Block []string `yaml:"block"`
	}

	ChannelSecurityConfig struct {
		Policy        consts.SecurityPolicy `yaml:"policy"`
		WelcomeWindow int                   `yaml:"welcome_window"`
		MaxResp       int                   `yaml:"max_resp"`
		CustomText    string                `yaml:"custom_text"`
	}

	ProviderConfig struct {
		ID     string         `yaml:"-"`
		Type   string         `yaml:"type"` // openai, anthropic, gemini, ollama, qwen
		Config map[string]any `yaml:"config"`
	}
)

// UpdateByName .
func (c *Config) UpdateByName(name string, value any) error {
	if c == nil {
		return fmt.Errorf("config cannot be nil")
	}

	normalizedName := strings.ToLower(strings.TrimSpace(name))
	if normalizedName == "" {
		return fmt.Errorf("name is required")
	}

	switch normalizedName {
	case "config":
		typed, ok := value.(*Config)
		if !ok || typed == nil {
			return fmt.Errorf("name 'config' requires *Config")
		}
		*c = *typed
	case "gateway":
		typed, ok := value.(*GatewayConfig)
		if !ok || typed == nil {
			return fmt.Errorf("name 'gateway' requires *GatewayConfig")
		}
		c.Gateway = *typed
	case "logging":
		typed, ok := value.(*LoggingConfig)
		if !ok || typed == nil {
			return fmt.Errorf("name 'logging' requires *LoggingConfig")
		}
		c.Logging = *typed
	case "cron":
		typed, ok := value.(*CronjobConfig)
		if !ok || typed == nil {
			return fmt.Errorf("name 'cron' requires *CronjobConfig")
		}
		c.Cronjob = *typed
	case "providers":
		typed, ok := value.(*map[string]ProviderConfig)
		if !ok || typed == nil {
			return fmt.Errorf("name 'providers' requires *map[string]ProviderConfig")
		}
		next := make(map[string]ProviderConfig, len(*typed))
		for k, v := range *typed {
			next[k] = v
		}
		c.Providers = next
	case "agents":
		typed, ok := value.(*map[string]AgentConfig)
		if !ok || typed == nil {
			return fmt.Errorf("name 'agents' requires *map[string]AgentConfig")
		}
		next := make(map[string]AgentConfig, len(*typed))
		for k, v := range *typed {
			next[k] = v
		}
		c.Agents = next
	case "channels":
		typed, ok := value.(*map[string]ChannelConfig)
		if !ok || typed == nil {
			return fmt.Errorf("name 'channels' requires *map[string]ChannelConfig")
		}
		next := make(map[string]ChannelConfig, len(*typed))
		for k, v := range *typed {
			next[k] = v
		}
		c.Channels = next
	default:
		return fmt.Errorf("unsupported config name: %s", name)
	}

	return nil
}

// Clone .
func (c *Config) Clone() (*Config, error) {
	if c == nil {
		return nil, fmt.Errorf("config is nil")
	}

	raw, err := sonic.Marshal(c)
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}

	var cloned Config
	if err := sonic.Unmarshal(raw, &cloned); err != nil {
		return nil, fmt.Errorf("unmarshal config clone: %w", err)
	}

	return &cloned, nil
}

// Hash .
func (c *Config) Hash() string {
	json := sonic.Config{SortMapKeys: true, UseNumber: true}.Froze()
	raw, _ := json.Marshal(c)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
