package ark

import (
	"context"
	"fmt"
	"sync"

	arkmodel "github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/tgifai/friday/internal/provider"
)

var _ provider.Provider = (*Provider)(nil)

type Provider struct {
	config   Config
	modelMap map[string]model.ToolCallingChatModel
	tools    []*schema.ToolInfo
	mu       sync.RWMutex
}

func (p *Provider) RegisterTools(tools []*schema.ToolInfo) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.tools = tools
}

func NewProvider(_ context.Context, id string, cfgMap map[string]any) (*Provider, error) {
	cfg, err := ParseConfig(id, cfgMap)
	if err != nil {
		return nil, fmt.Errorf("parse ark config: %w", err)
	}

	return &Provider{
		config:   *cfg,
		modelMap: make(map[string]model.ToolCallingChatModel, 4),
	}, nil
}

func (p *Provider) ID() string {
	return p.config.ID
}

func (p *Provider) Type() provider.Type {
	return provider.Ark
}

func (p *Provider) IsAvailable() bool {
	return p.config.APIKey != "" || (p.config.AccessKey != "" && p.config.SecretKey != "")
}

func (p *Provider) Close() error {
	return nil
}

func (p *Provider) ListModels(_ context.Context) ([]provider.ModelInfo, error) {
	return []provider.ModelInfo{
		{
			ID:       p.config.DefaultModel,
			Name:     p.config.DefaultModel,
			Provider: provider.Ark,
		},
	}, nil
}

func (p *Provider) Generate(ctx context.Context, modelName string, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	if modelName == "" {
		modelName = p.config.DefaultModel
	}
	ctx, cancel := context.WithTimeout(ctx, p.config.Timeout)
	defer cancel()

	chatModel, err := p.getOrCreateModel(ctx, modelName)
	if err != nil {
		return nil, fmt.Errorf("failed to get chat model for %s: %w", modelName, err)
	}

	opts = p.appendSessionCacheOpts(input, opts)

	resp, err := chatModel.Generate(ctx, input, opts...)
	if err != nil {
		return nil, fmt.Errorf("ark API call failed: %w", err)
	}
	return resp, nil
}

func (p *Provider) Stream(ctx context.Context, modelName string, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	if modelName == "" {
		modelName = p.config.DefaultModel
	}
	ctx, cancel := context.WithTimeout(ctx, p.config.Timeout)
	defer cancel()

	chatModel, err := p.getOrCreateModel(ctx, modelName)
	if err != nil {
		return nil, fmt.Errorf("failed to get chat model for %s: %w", modelName, err)
	}

	opts = p.appendSessionCacheOpts(input, opts)

	streamReader, err := chatModel.Stream(ctx, input, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create stream: %w", err)
	}
	return streamReader, nil
}

// appendSessionCacheOpts enables SDK-managed session caching and handles
// cache invalidation when the agent layer detects system prompt changes.
func (p *Provider) appendSessionCacheOpts(input []*schema.Message, opts []model.Option) []model.Option {
	if !p.config.SessionCacheEnabled {
		return opts
	}

	if shouldInvalidateCache(input) {
		_ = arkmodel.InvalidateMessageCaches(input)
	}

	opts = append(opts, arkmodel.WithCache(&arkmodel.CacheOption{
		SessionCache: &arkmodel.SessionCacheConfig{
			EnableCache: true,
		},
	}))
	return opts
}

func shouldInvalidateCache(msgs []*schema.Message) bool {
	for _, m := range msgs {
		if m.Role == schema.System && m.Extra[provider.CacheInvalidate] == true {
			return true
		}
	}
	return false
}

func (p *Provider) getOrCreateModel(ctx context.Context, modelName string) (model.ToolCallingChatModel, error) {
	p.mu.RLock()
	if m, exists := p.modelMap[modelName]; exists {
		p.mu.RUnlock()
		return m, nil
	}
	p.mu.RUnlock()

	p.mu.Lock()
	defer p.mu.Unlock()

	if m, exists := p.modelMap[modelName]; exists {
		return m, nil
	}

	timeout := p.config.Timeout
	retries := p.config.MaxRetries

	arkCfg := &arkmodel.ResponsesAPIConfig{
		Model:      modelName,
		APIKey:     p.config.APIKey,
		AccessKey:  p.config.AccessKey,
		SecretKey:  p.config.SecretKey,
		Timeout:    &timeout,
		RetryTimes: &retries,
	}
	if p.config.BaseURL != "" {
		arkCfg.BaseURL = p.config.BaseURL
	}
	if p.config.Temperature > 0 {
		arkCfg.Temperature = &p.config.Temperature
	}

	chatModel, err := arkmodel.NewResponsesAPIChatModel(ctx, arkCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create ark responses API model for %s: %w", modelName, err)
	}

	cm, err := chatModel.WithTools(p.tools)
	if err != nil {
		return nil, fmt.Errorf("failed to bind tools with model for %s: %w", modelName, err)
	}

	p.modelMap[modelName] = cm
	return cm, nil
}
