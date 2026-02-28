package ark

import (
	"errors"
	"fmt"
	"time"

	"github.com/bytedance/gg/gconv"
)

const defaultPrefixCacheTTL = 60 * 60 * 8 // 8hrs

type Config struct {
	ID           string
	APIKey       string
	AccessKey    string
	SecretKey    string
	BaseURL      string
	DefaultModel string
	Timeout      time.Duration
	MaxRetries   int
	Temperature  float32

	// PrefixCacheEnabled controls whether prefix caching is automatically
	// applied. When true (default), the provider creates a server-side prefix
	// cache from system messages on the first Generate/Stream call and reuses
	// the cached response ID on subsequent calls via the ResponsesAPI.
	PrefixCacheEnabled bool
	// PrefixCacheTTL is the time-to-live in seconds for the prefix cache.
	// Default: 8hrs.
	PrefixCacheTTL int
}

func (c *Config) Validate() error {
	if c.ID == "" {
		return errors.New("provider ID cannot be empty")
	}
	if c.APIKey == "" && (c.AccessKey == "" || c.SecretKey == "") {
		return errors.New("either api_key or access_key+secret_key is required")
	}
	if c.DefaultModel == "" {
		return errors.New("default_model (endpoint ID) is required")
	}
	if c.Timeout <= 0 {
		return errors.New("timeout must be positive")
	}
	if c.MaxRetries < 0 {
		return errors.New("max_retries must be non-negative")
	}
	if c.PrefixCacheTTL <= 0 {
		c.PrefixCacheTTL = defaultPrefixCacheTTL
	}
	return nil
}

func ParseConfig(id string, configMap map[string]any) (*Config, error) {
	config := &Config{
		ID:                 id,
		PrefixCacheEnabled: true, // default on
	}

	apiKey := gconv.To[string](configMap["api_key"])
	if apiKey == "" {
		if secretKey := gconv.To[string](configMap["secret_key"]); secretKey != "" {
			apiKey = secretKey
		}
	}
	config.APIKey = apiKey

	config.AccessKey = gconv.To[string](configMap["access_key"])
	config.SecretKey = gconv.To[string](configMap["secret_key"])

	if baseURL := gconv.To[string](configMap["base_url"]); baseURL != "" {
		config.BaseURL = baseURL
	}

	if defaultModel := gconv.To[string](configMap["default_model"]); defaultModel != "" {
		config.DefaultModel = defaultModel
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

	config.Temperature = float32(gconv.To[float64](configMap["temperature"]))

	// prefix_cache_enabled: defaults to true; set false to disable.
	if v, ok := configMap["prefix_cache_enabled"]; ok {
		config.PrefixCacheEnabled = gconv.To[bool](v)
	}

	if ttl := gconv.To[int](configMap["prefix_cache_ttl"]); ttl > 0 {
		config.PrefixCacheTTL = ttl
	} else {
		config.PrefixCacheTTL = defaultPrefixCacheTTL
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid ark config: %w", err)
	}

	return config, nil
}
