package http

import (
	"fmt"

	"github.com/bytedance/gg/gconv"

	"github.com/tgifai/friday/internal/channel"
)

type Config struct {
	// APIKey is an optional bearer token for authenticating incoming requests.
	// When set, requests must include "Authorization: Bearer <api_key>".
	APIKey string
}

func (c *Config) Validate() error {
	return nil
}

func (c *Config) GetType() channel.Type {
	return channel.HTTP
}

func ParseConfig(configMap map[string]interface{}) (*Config, error) {
	cfg := &Config{}
	cfg.APIKey = gconv.To[string](configMap["api_key"])

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid http config: %w", err)
	}
	return cfg, nil
}
