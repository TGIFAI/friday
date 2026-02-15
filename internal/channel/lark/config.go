package lark

import (
	"errors"
	"fmt"

	"github.com/bytedance/gg/gconv"

	"github.com/tgifai/friday/internal/channel"
)

type Config struct {
	AppID             string // Lark App ID (required)
	AppSecret         string // Lark App Secret (required)
	VerificationToken string // Event subscription verification token (required)
	EncryptKey        string // Event subscription encryption key (optional)
}

func (c *Config) Validate() error {
	if c.AppID == "" {
		return errors.New("lark app_id cannot be empty")
	}
	if c.AppSecret == "" {
		return errors.New("lark app_secret cannot be empty")
	}
	if c.VerificationToken == "" {
		return errors.New("lark verification_token cannot be empty")
	}
	return nil
}

func (c *Config) GetType() channel.Type {
	return channel.Lark
}

func ParseConfig(configMap map[string]interface{}) (*Config, error) {
	config := &Config{}

	config.AppID = gconv.To[string](configMap["app_id"])
	if config.AppID == "" {
		return nil, errors.New("lark app_id is required")
	}

	config.AppSecret = gconv.To[string](configMap["app_secret"])
	if config.AppSecret == "" {
		return nil, errors.New("lark app_secret is required")
	}

	config.VerificationToken = gconv.To[string](configMap["verification_token"])
	if config.VerificationToken == "" {
		return nil, errors.New("lark verification_token is required")
	}

	config.EncryptKey = gconv.To[string](configMap["encrypt_key"])

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid lark config: %w", err)
	}

	return config, nil
}
