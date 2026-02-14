package sandbox

import (
	"fmt"
	"strings"
	"sync"
)

const (
	BackboneLocal   = "local"
	BackboneGoJudge = "go_judge"
)

type BackboneBuilder func(workspace string, cfg SandboxConfig) (Executor, error)

var (
	backboneBuilders = map[string]BackboneBuilder{
		BackboneLocal: func(workspace string, cfg SandboxConfig) (Executor, error) {
			return NewLocalExecutor(workspace), nil
		},
		BackboneGoJudge: func(workspace string, cfg SandboxConfig) (Executor, error) {
			return NewGoJudgeExecutor(workspace, cfg.GoJudge), nil
		},
	}
	backboneMu sync.RWMutex
)

func RegisterBackbone(name string, builder BackboneBuilder) error {
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" {
		return fmt.Errorf("backbone name is required")
	}
	if builder == nil {
		return fmt.Errorf("backbone builder cannot be nil")
	}

	backboneMu.Lock()
	defer backboneMu.Unlock()
	if _, exists := backboneBuilders[key]; exists {
		return fmt.Errorf("backbone already registered: %s", key)
	}
	backboneBuilders[key] = builder
	return nil
}

func NewExecutorForTool(workspace string, cfg SandboxConfig, toolName string) (Executor, bool, error) {
	if !cfg.Enable || !cfg.AppliesToTool(toolName) {
		return nil, false, nil
	}

	runtimeName := strings.ToLower(strings.TrimSpace(cfg.Runtime))
	if runtimeName == "" {
		runtimeName = BackboneLocal
	}

	backboneMu.RLock()
	builder, ok := backboneBuilders[runtimeName]
	backboneMu.RUnlock()
	if !ok {
		return nil, false, fmt.Errorf("unsupported sandbox backbone: %s", runtimeName)
	}

	executor, err := builder(workspace, cfg)
	if err != nil {
		return nil, false, err
	}
	if executor == nil {
		return nil, false, fmt.Errorf("sandbox backbone %s returned nil executor", runtimeName)
	}
	return executor, true, nil
}
