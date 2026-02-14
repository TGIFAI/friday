package tool

import (
	"context"
	"fmt"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/cloudwego/eino/schema"

	"github.com/tgifai/friday/internal/pkg/logs"
)

type Registry struct {
	tools map[string]Tool
	mu    sync.RWMutex
}

func NewRegistry(tools ...Tool) *Registry {
	reg := &Registry{
		tools: make(map[string]Tool, 32),
	}
	for _, t := range tools {
		reg.tools[t.Name()] = t
	}
	return reg
}

func (r *Registry) Register(tool Tool) error {
	if tool == nil {
		return fmt.Errorf("tool cannot be nil")
	}

	name := tool.Name()
	if name == "" {
		return fmt.Errorf("tool name cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("tool already registered: %s", name)
	}

	r.tools[name] = tool
	logs.Info("[tool:registry] registered tool: %s", name)
	return nil
}

func (r *Registry) Get(name string) (Tool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tool, exists := r.tools[name]
	if !exists {
		return nil, fmt.Errorf("tool not found: %s", name)
	}

	return tool, nil
}

func (r *Registry) List() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tools := make([]Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		tools = append(tools, tool)
	}

	return tools
}

func (r *Registry) ListToolInfos() []*schema.ToolInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	toolInfos := make([]*schema.ToolInfo, 0, len(r.tools))
	for _, tool := range r.tools {
		toolInfos = append(toolInfos, tool.ToolInfo())
	}

	return toolInfos
}

func (r *Registry) Execute(ctx context.Context, toolName string, args map[string]interface{}) (interface{}, error) {
	tool, err := r.Get(toolName)
	if err != nil {
		return nil, err
	}

	return tool.Execute(ctx, args)
}

func (r *Registry) ExecuteToolCall(ctx context.Context, toolCall *schema.ToolCall) (interface{}, error) {
	if toolCall == nil {
		return nil, fmt.Errorf("tool call cannot be nil")
	}

	toolName := toolCall.Function.Name
	if toolName == "" {
		return nil, fmt.Errorf("tool name is required")
	}

	var args map[string]interface{}
	if toolCall.Function.Arguments != "" {
		if err := sonic.UnmarshalString(toolCall.Function.Arguments, &args); err != nil {
			return nil, fmt.Errorf("failed to parse tool arguments: %w", err)
		}
	} else {
		args = make(map[string]interface{})
	}

	return r.Execute(ctx, toolName, args)
}

type Tool interface {
	Name() string

	Description() string

	ToolInfo() *schema.ToolInfo

	Execute(ctx context.Context, args map[string]interface{}) (interface{}, error)
}
