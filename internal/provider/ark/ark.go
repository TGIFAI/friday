package ark

import (
	"context"
	"fmt"
	"sync"

	arkmodel "github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/tgifai/friday/internal/pkg/logs"
	"github.com/tgifai/friday/internal/provider"
)

var _ provider.Provider = (*Provider)(nil)

type Provider struct {
	config      Config
	modelMap    map[string]model.ToolCallingChatModel
	rawModelMap map[string]*arkmodel.ResponsesAPIChatModel
	tools       []*schema.ToolInfo
	// cacheResponseIDs stores prefix cache response IDs per model name.
	// Populated lazily on the first Generate/Stream call when PrefixCacheEnabled is true.
	cacheResponseIDs map[string]string
	prefixCacheOnce  map[string]*sync.Once
	mu               sync.RWMutex
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
		config:           *cfg,
		modelMap:         make(map[string]model.ToolCallingChatModel, 4),
		rawModelMap:      make(map[string]*arkmodel.ResponsesAPIChatModel, 4),
		cacheResponseIDs: make(map[string]string, 4),
		prefixCacheOnce:  make(map[string]*sync.Once, 4),
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

	opts = p.appendPrefixCacheOpts(ctx, modelName, input, opts)

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

	opts = p.appendPrefixCacheOpts(ctx, modelName, input, opts)

	streamReader, err := chatModel.Stream(ctx, input, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create stream: %w", err)
	}
	return streamReader, nil
}

// appendPrefixCacheOpts auto-creates a prefix cache from system messages on the
// first call and injects WithCache (ResponsesAPI) on every subsequent call.
func (p *Provider) appendPrefixCacheOpts(ctx context.Context, modelName string, input []*schema.Message, opts []model.Option) []model.Option {
	if !p.config.PrefixCacheEnabled {
		return opts
	}

	p.mu.RLock()
	rawModel := p.rawModelMap[modelName]
	p.mu.RUnlock()
	if rawModel == nil {
		return opts
	}

	responseID := p.getOrCreatePrefixCache(ctx, rawModel, modelName, input)
	if responseID == "" {
		return opts
	}

	opts = append(opts, arkmodel.WithCache(&arkmodel.CacheOption{
		APIType:                arkmodel.ResponsesAPI,
		HeadPreviousResponseID: &responseID,
	}))
	return opts
}

// getOrCreatePrefixCache returns an existing cached response ID or attempts to
// create a new prefix cache from system messages via the ResponsesAPI.
// Uses sync.Once per model to guarantee exactly one CreatePrefixCache call.
func (p *Provider) getOrCreatePrefixCache(ctx context.Context, chatModel *arkmodel.ResponsesAPIChatModel, modelName string, input []*schema.Message) string {
	// Fast path: cache already exists.
	p.mu.RLock()
	if id, ok := p.cacheResponseIDs[modelName]; ok {
		p.mu.RUnlock()
		return id
	}
	p.mu.RUnlock()

	// Get or create a sync.Once for this model.
	p.mu.Lock()
	if id, ok := p.cacheResponseIDs[modelName]; ok {
		p.mu.Unlock()
		return id
	}
	once, ok := p.prefixCacheOnce[modelName]
	if !ok {
		once = &sync.Once{}
		p.prefixCacheOnce[modelName] = once
	}
	p.mu.Unlock()

	// Exactly one goroutine executes CreatePrefixCache; others block here.
	once.Do(func() {
		systemMsgs := extractSystemMessages(input)
		if len(systemMsgs) == 0 {
			return
		}
		info, err := chatModel.CreatePrefixCache(ctx, systemMsgs, p.config.PrefixCacheTTL)
		if err != nil {
			logs.CtxWarn(ctx, "[ark:%s] prefix cache creation failed (input may be < 1024 tokens): %v", p.config.ID, err)
			return
		}
		p.mu.Lock()
		p.cacheResponseIDs[modelName] = info.ResponseID
		p.mu.Unlock()
		logs.CtxInfo(ctx, "[ark:%s] prefix cache created for model %s, response_id=%s, cached_tokens=%d",
			p.config.ID, modelName, info.ResponseID, info.Usage.PromptTokenDetails.CachedTokens)
	})

	p.mu.RLock()
	id := p.cacheResponseIDs[modelName]
	p.mu.RUnlock()
	return id
}

func extractSystemMessages(msgs []*schema.Message) []*schema.Message {
	var result []*schema.Message
	for _, m := range msgs {
		if m.Role == schema.System && m.Extra[provider.L0Cache] == true {
			result = append(result, m)
		}
	}
	return result
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

	p.rawModelMap[modelName] = chatModel

	cm, err := chatModel.WithTools(p.tools)
	if err != nil {
		return nil, fmt.Errorf("failed to bind tools with model for %s: %w", modelName, err)
	}

	p.modelMap[modelName] = cm
	return cm, nil
}
