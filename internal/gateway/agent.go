package gateway

import (
	"errors"
	"fmt"
	"sync"

	"github.com/bytedance/gg/gmap"

	"github.com/tgifai/friday/internal/agent"
)

type agentRegistry struct {
	agents map[string]*agent.Agent
	mu     sync.RWMutex
}

func (r *agentRegistry) Register(agent *agent.Agent) error {
	if agent == nil {
		return errors.New("agent cannot be nil")
	}
	if agent.ID() == "" {
		return errors.New("agent ID cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.agents[agent.ID()] != nil {
		return fmt.Errorf("agent already registered: %s", agent.ID())
	}

	r.agents[agent.ID()] = agent
	return nil
}

func (r *agentRegistry) Get(agentID string) (*agent.Agent, error) {
	if agentID == "" {
		return nil, errors.New("agent ID cannot be empty")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	ag, exists := r.agents[agentID]
	if !exists {
		return nil, fmt.Errorf("agent not found: %s", agentID)
	}

	return ag, nil
}

func (r *agentRegistry) List() []*agent.Agent {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return gmap.ToSlice(r.agents, func(k string, v *agent.Agent) *agent.Agent { return v })
}
