package agent

import (
	"github.com/tgifai/friday/internal/agent/skill"
	"github.com/tgifai/friday/internal/config"
)

type Agent struct {
	id          string
	name        string
	description string

	//tools  *tool.Registry
	skills *skill.Registry
	//sess   *session.Manager

	config    config.AgentConfig
	workspace string
}
