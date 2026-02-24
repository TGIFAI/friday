package telegram

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bytedance/gg/gconv"

	"github.com/tgifai/friday/internal/channel"
)

type Config struct {
	Token       string        // Telegram Bot Token (required)
	Mode        string        // "polling" (default) or "webhook"
	WebhookURL  string        // External base URL for webhook (required in webhook mode, e.g. "https://example.com")
	SecretToken string        // Secret token for webhook verification (optional, webhook mode)
	PollTimeout time.Duration // Long-polling timeout (polling mode only, default 30s)
	MaxWorkers  int           // Concurrent update workers (default 10)
}

func (c *Config) Validate() error {
	if c.Token == "" {
		return errors.New("telegram bot token cannot be empty")
	}
	if c.Mode != "polling" && c.Mode != "webhook" {
		return fmt.Errorf("telegram mode must be \"polling\" or \"webhook\", got %q", c.Mode)
	}
	if c.Mode == "webhook" && c.WebhookURL == "" {
		return errors.New("telegram webhook_url cannot be empty in webhook mode")
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

	config.Mode = strings.ToLower(gconv.To[string](configMap["mode"]))
	if config.Mode == "" {
		config.Mode = "polling"
	}

	config.WebhookURL = strings.TrimRight(gconv.To[string](configMap["webhook_url"]), "/")
	config.SecretToken = gconv.To[string](configMap["secret_token"])

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
