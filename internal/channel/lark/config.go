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
	Mode              string // "webhook" (default) or "ws"
	VerificationToken string // Event subscription verification token (webhook mode required)
	EncryptKey        string // Event subscription encryption key (webhook mode optional)
}

func (c *Config) Validate() error {
	if c.AppID == "" {
		return errors.New("lark app_id cannot be empty")
	}
	if c.AppSecret == "" {
		return errors.New("lark app_secret cannot be empty")
	}
	if c.Mode != "webhook" && c.Mode != "ws" {
		return fmt.Errorf("lark mode must be \"webhook\" or \"ws\", got %q", c.Mode)
	}
	if c.Mode == "webhook" && c.VerificationToken == "" {
		return errors.New("lark verification_token cannot be empty in webhook mode")
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

	config.Mode = gconv.To[string](configMap["mode"])
	if config.Mode == "" {
		config.Mode = "webhook"
	}

	config.VerificationToken = gconv.To[string](configMap["verification_token"])
	config.EncryptKey = gconv.To[string](configMap["encrypt_key"])

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid lark config: %w", err)
	}

	return config, nil
}
