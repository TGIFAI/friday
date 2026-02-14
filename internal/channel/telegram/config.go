package telegram

import (
	"errors"
	"fmt"
	"time"

	"github.com/bytedance/gg/gconv"

	"github.com/tgifai/friday/internal/channel"
	"github.com/tgifai/friday/internal/config"
)

type Config struct {
	Token         string // Telegram Bot Token
	WebhookURL    string
	WebhookPort   int
	PollTimeout   time.Duration
	MaxWorkers    int
	AllowedUsers  []int64
	AllowedGroups []int64
	Security      config.ChannelSecurityConfig
	ACL           map[string]config.ChannelACLConfig
	ConfigPath    string
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
	chCfg := config.ChannelConfig{
		ID:       "telegram-config",
		Type:     string(channel.Telegram),
		ACL:      c.ACL,
		Security: c.Security,
	}
	if err := chCfg.Validate(); err != nil {
		return fmt.Errorf("invalid telegram pairing settings: %w", err)
	}
	c.Security = chCfg.Security
	c.ACL = chCfg.ACL
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

	if allowedUsersRaw, ok := configMap["allowed_users"].([]interface{}); ok && len(allowedUsersRaw) > 0 {
		config.AllowedUsers = make([]int64, 0, len(allowedUsersRaw))
		for _, u := range allowedUsersRaw {
			userID := gconv.To[int64](u)
			if userID == 0 {
				return nil, fmt.Errorf("invalid user ID: %v", u)
			}
			config.AllowedUsers = append(config.AllowedUsers, userID)
		}
	}

	if allowedGroupsRaw, ok := configMap["allowed_groups"].([]interface{}); ok && len(allowedGroupsRaw) > 0 {
		config.AllowedGroups = make([]int64, 0, len(allowedGroupsRaw))
		for _, g := range allowedGroupsRaw {
			groupID := gconv.To[int64](g)
			if groupID == 0 {
				return nil, fmt.Errorf("invalid group ID: %v", g)
			}
			config.AllowedGroups = append(config.AllowedGroups, groupID)
		}
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid telegram config: %w", err)
	}

	return config, nil
}
