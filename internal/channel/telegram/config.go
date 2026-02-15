package telegram

import (
	"errors"
	"fmt"
	"time"

	"github.com/bytedance/gg/gconv"

	"github.com/tgifai/friday/internal/channel"
)

type Config struct {
	Token       string // Telegram Bot Token
	WebhookURL  string
	WebhookPort int
	PollTimeout time.Duration
	MaxWorkers  int
}

func (c *Config) Validate() error {
	if c.Token == "" {
		return errors.New("telegram bot token cannot be empty")
	}
	if c.PollTimeout == 0 {
		c.PollTimeout = 30 * time.Second
	}
	if c.MaxWorkers <= 0 {
		c.MaxWorkers = 10
	}
	return nil
}

func (c *Config) GetType() channel.Type {
	return channel.Telegram
}

func ParseConfig(configMap map[string]interface{}) (*Config, error) {
	config := &Config{}

	token := gconv.To[string](configMap["token"])
	if token == "" {
		return nil, errors.New("telegram token is required")
	}
	config.Token = token

	if webhookURL := gconv.To[string](configMap["webhook_url"]); webhookURL != "" {
		config.WebhookURL = webhookURL
	}
	if webhookPort := gconv.To[int](configMap["webhook_port"]); webhookPort > 0 {
		config.WebhookPort = webhookPort
	}

	if pollTimeout := gconv.To[int](configMap["poll_timeout"]); pollTimeout > 0 {
		config.PollTimeout = time.Duration(pollTimeout) * time.Second
	} else {
		config.PollTimeout = 30 * time.Second
	}

	if maxWorkers := gconv.To[int](configMap["max_workers"]); maxWorkers > 0 {
		config.MaxWorkers = maxWorkers
	} else {
		config.MaxWorkers = 10
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid telegram config: %w", err)
	}

	return config, nil
}
