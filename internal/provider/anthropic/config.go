package anthropic

import (
	"errors"
	"fmt"
	"time"

	"github.com/bytedance/gg/gconv"
)

type Config struct {
	ID           string
	APIKey       string
	BaseURL      string
	DefaultModel string
	MaxTokens    int
	Timeout      time.Duration
	MaxRetries   int
}

func (c *Config) Validate() error {
	if c.ID == "" {
		return errors.New("provider ID cannot be empty")
	}
	if c.APIKey == "" {
		return errors.New("API key cannot be empty")
	}
	if c.BaseURL == "" {
		c.BaseURL = "https://api.anthropic.com"
	}
	if c.DefaultModel == "" {
		c.DefaultModel = "claude-3-5-sonnet-20241022"
	}
	if c.MaxTokens <= 0 {
		c.MaxTokens = 4096
	}
	if c.Timeout == 0 {
		c.Timeout = 60 * time.Second
	}
	if c.MaxRetries < 0 {
		c.MaxRetries = 3
	}
	return nil
}

func ParseConfig(id string, configMap map[string]interface{}) (*Config, error) {
	config := &Config{
		ID: id,
	}

	apiKey := gconv.To[string](configMap["api_key"])
	if apiKey == "" {

		if secretKey := gconv.To[string](configMap["secret_key"]); secretKey != "" {
			apiKey = secretKey
		}
	}
	if apiKey == "" {
		return nil, errors.New("anthropic api_key is required")
	}
	config.APIKey = apiKey

	if baseURL := gconv.To[string](configMap["base_url"]); baseURL != "" {
		config.BaseURL = baseURL
	} else {
		config.BaseURL = "https://api.anthropic.com"
	}

	if defaultModel := gconv.To[string](configMap["default_model"]); defaultModel != "" {
		config.DefaultModel = defaultModel
	} else {
		config.DefaultModel = "claude-3-5-sonnet-20241022"
	}

	if maxTokens := gconv.To[int](configMap["max_tokens"]); maxTokens > 0 {
		config.MaxTokens = maxTokens
	} else {
		config.MaxTokens = 4096
	}

	if timeout := gconv.To[int](configMap["timeout"]); timeout > 0 {
		config.Timeout = time.Duration(timeout) * time.Second
	} else {
		config.Timeout = 60 * time.Second
	}

	if maxRetries := gconv.To[int](configMap["max_retries"]); maxRetries > 0 {
		config.MaxRetries = maxRetries
	} else {
		config.MaxRetries = 3
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid anthropic config: %w", err)
	}

	return config, nil
}
