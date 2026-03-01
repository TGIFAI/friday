package ark

import (
	"errors"
	"fmt"
	"time"

	"github.com/bytedance/gg/gconv"
)

type Config struct {
	ID           string
	APIKey       string
	AccessKey    string
	SecretKey string
	BaseURL   string
	Timeout   time.Duration
	MaxRetries   int
	// SessionCacheEnabled controls whether session caching is automatically
	// applied. When true, the SDK stores both inputs and responses for each
	// conversation turn and trims already-cached messages on subsequent calls.
	// Default: false (opt-in).
	SessionCacheEnabled bool

	// SessionCacheTTL is the cache time-to-live in seconds.
	// Maximum allowed by the API is 259200 (3 days).
	// Default: 7200 (2 hours).
	SessionCacheTTL int
}

func (c *Config) Validate() error {
	if c.ID == "" {
		return errors.New("provider ID cannot be empty")
	}
	if c.APIKey == "" && (c.AccessKey == "" || c.SecretKey == "") {
		return errors.New("either api_key or access_key+secret_key is required")
	}
	if c.Timeout <= 0 {
		return errors.New("timeout must be positive")
	}
	if c.MaxRetries < 0 {
		return errors.New("max_retries must be non-negative")
	}
	return nil
}

func ParseConfig(id string, configMap map[string]any) (*Config, error) {
	config := &Config{
		ID: id,
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

	if v, ok := configMap["session_cache_enabled"]; ok {
		config.SessionCacheEnabled = gconv.To[bool](v)
	}

	if ttl := gconv.To[int](configMap["session_cache_ttl"]); ttl > 0 {
		config.SessionCacheTTL = ttl
	} else {
		config.SessionCacheTTL = 7200 // 2 hours
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid ark config: %w", err)
	}

	return config, nil
}
