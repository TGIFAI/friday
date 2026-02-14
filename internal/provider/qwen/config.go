package qwen

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
		c.BaseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	}
	if c.DefaultModel == "" {
		c.DefaultModel = "qwen-plus"
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
		return nil, errors.New("qwen api_key is required")
	}
	config.APIKey = apiKey

	if baseURL := gconv.To[string](configMap["base_url"]); baseURL != "" {
		config.BaseURL = baseURL
	} else {
		config.BaseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	}

	if defaultModel := gconv.To[string](configMap["default_model"]); defaultModel != "" {
		config.DefaultModel = defaultModel
	} else {
		config.DefaultModel = "qwen-plus"
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
		return nil, fmt.Errorf("invalid qwen config: %w", err)
	}

	return config, nil
}
