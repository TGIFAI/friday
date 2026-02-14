package config

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tgifai/friday/internal/consts"
)

const (
	defaultPairingWelcomeWindowSec = 300
	defaultPairingMaxResp          = 3
)

// Validate .
func (c *Config) Validate() error {
	if c == nil {
		return errors.New("config cannot be nil")
	}

	if c.Cronjob.Enabled == nil {
		enabled := true
		c.Cronjob.Enabled = &enabled
	}
	if c.Cronjob.MaxConcurrentRuns <= 0 {
		c.Cronjob.MaxConcurrentRuns = 1
	}
	if c.Cronjob.JobTimeoutSec <= 0 {
		c.Cronjob.JobTimeoutSec = 300
	}
	c.Cronjob.Store = strings.TrimSpace(c.Cronjob.Store)
	if c.Cronjob.Store == "" {
		c.Cronjob.Store = filepath.Join("output", "cron", "jobs.json")
	}
	c.Cronjob.SessionRetention = strings.TrimSpace(c.Cronjob.SessionRetention)
	if c.Cronjob.SessionRetention == "" {
		c.Cronjob.SessionRetention = "24h"
	}

	normalizedProviders := make(map[string]ProviderConfig, len(c.Providers))
	for key, one := range c.Providers {
		providerID := strings.TrimSpace(key)
		if providerID == "" {
			return errors.New("provider id cannot be empty")
		}
		one.ID = providerID
		normalizedProviders[providerID] = one
	}
	c.Providers = normalizedProviders

	normalizedAgents := make(map[string]AgentConfig, len(c.Agents))
	for key, one := range c.Agents {
		agentID := strings.TrimSpace(key)
		if agentID == "" {
			return errors.New("agent id cannot be empty")
		}
		one.ID = agentID
		normalizedAgents[agentID] = one
	}
	c.Agents = normalizedAgents

	normalizedChannels := make(map[string]ChannelConfig, len(c.Channels))
	for key, one := range c.Channels {
		channelID := strings.TrimSpace(key)
		if channelID == "" {
			return errors.New("channel id cannot be empty")
		}
		one.ID = channelID

		if err := one.Validate(); err != nil {
			return fmt.Errorf("channels[%s] validation failed: %w", channelID, err)
		}
		normalizedChannels[channelID] = one
	}
	c.Channels = normalizedChannels
	return nil
}

func (c *ChannelConfig) Validate() error {
	if c == nil {
		return errors.New("channel config cannot be nil")
	}

	securityEmpty := c.Security.Policy == "" &&
		c.Security.WelcomeWindow == 0 &&
		c.Security.MaxResp == 0 &&
		strings.TrimSpace(c.Security.CustomText) == ""
	if securityEmpty && len(c.ACL) == 0 {
		return nil
	}

	if c.Security.Policy == "" {
		c.Security.Policy = consts.SecurityPolicyWelcome
	}
	if c.Security.WelcomeWindow <= 0 {
		c.Security.WelcomeWindow = defaultPairingWelcomeWindowSec
	}
	if c.Security.MaxResp <= 0 {
		c.Security.MaxResp = defaultPairingMaxResp
	}
	c.Security.CustomText = strings.TrimSpace(c.Security.CustomText)

	switch c.Security.Policy {
	case consts.SecurityPolicyWelcome, consts.SecurityPolicySilent, consts.SecurityPolicyCustom:
	default:
		return fmt.Errorf("invalid security.policy: %s", c.Security.Policy)
	}

	if c.Security.WelcomeWindow <= 0 {
		return errors.New("security.welcome_window must be greater than 0")
	}
	if c.Security.MaxResp <= 0 {
		return errors.New("security.max_resp must be greater than 0")
	}
	if c.Security.Policy == consts.SecurityPolicyCustom && c.Security.CustomText == "" {
		return errors.New("security.custom_text is required when security.policy=custom")
	}

	if len(c.ACL) == 0 {
		return nil
	}

	normalized := make(map[string]ChannelACLConfig, len(c.ACL))
	for key, one := range c.ACL {
		chatID := strings.TrimSpace(key)
		if chatID == "" {
			return errors.New("acl key cannot be empty")
		}
		if !strings.HasPrefix(chatID, "group:") && !strings.HasPrefix(chatID, "user:") {
			return fmt.Errorf("acl key must start with group: or user:, got %s", chatID)
		}

		normalizeList := func(in []string) []string {
			if len(in) == 0 {
				return nil
			}
			uniq := make(map[string]struct{}, len(in))
			out := make([]string, 0, len(in))
			for _, one := range in {
				one = strings.TrimSpace(one)
				if one == "" {
					continue
				}
				if _, ok := uniq[one]; ok {
					continue
				}
				uniq[one] = struct{}{}
				out = append(out, one)
			}
			sort.Strings(out)
			if len(out) == 0 {
				return nil
			}
			return out
		}

		one.Allow = normalizeList(one.Allow)
		one.Block = normalizeList(one.Block)
		normalized[chatID] = one
	}
	c.ACL = normalized
	return nil
}
