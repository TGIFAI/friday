package ark

import (
	"context"
	"fmt"
	"sync"
	"time"

	arkmodel "github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/tgifai/friday/internal/pkg/logs"
	"github.com/tgifai/friday/internal/provider"
)

var _ provider.Provider = (*Provider)(nil)

// prefixCacheEntry holds a per-model prefix cache state with expiry tracking.
// The entry-level mutex serialises CreatePrefixCache calls for a single model
// without blocking other models.
type prefixCacheEntry struct {
	mu         sync.Mutex
	responseID string
	expiresAt  time.Time
}

type Provider struct {
	config      Config
	modelMap    map[string]model.ToolCallingChatModel
	rawModelMap map[string]*arkmodel.ResponsesAPIChatModel
	tools       []*schema.ToolInfo
	prefixCache map[string]*prefixCacheEntry
	mu          sync.RWMutex
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
		config:      *cfg,
		modelMap:    make(map[string]model.ToolCallingChatModel, 4),
		rawModelMap: make(map[string]*arkmodel.ResponsesAPIChatModel, 4),
		prefixCache: make(map[string]*prefixCacheEntry, 4),
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

// getOrCreatePrefixCache returns a valid cached response ID for the given model,
// creating or refreshing the prefix cache as needed. It is safe for concurrent
// use: a per-model mutex serialises CreatePrefixCache calls so that different
// models never block each other.
func (p *Provider) getOrCreatePrefixCache(ctx context.Context, chatModel *arkmodel.ResponsesAPIChatModel, modelName string, input []*schema.Message) string {
	now := time.Now()

	// Hot path: entry exists and has not expired.
	p.mu.RLock()
	entry := p.prefixCache[modelName]
	if entry != nil && entry.responseID != "" && now.Before(entry.expiresAt) {
		p.mu.RUnlock()
		return entry.responseID
	}
	p.mu.RUnlock()

	// Ensure an entry struct exists for this model.
	p.mu.Lock()
	entry = p.prefixCache[modelName]
	if entry == nil {
		entry = &prefixCacheEntry{}
		p.prefixCache[modelName] = entry
	}
	p.mu.Unlock()

	// Per-model lock: only blocks goroutines targeting the same model.
	entry.mu.Lock()
	defer entry.mu.Unlock()

	// Re-check after acquiring the per-model lock (another goroutine may have refreshed).
	if entry.responseID != "" && time.Now().Before(entry.expiresAt) {
		return entry.responseID
	}

	systemMsgs := extractSystemMessages(input)
	if len(systemMsgs) == 0 {
		return entry.responseID
	}

	info, err := chatModel.CreatePrefixCache(ctx, systemMsgs, p.config.PrefixCacheTTL)
	if err != nil {
		logs.CtxWarn(ctx, "[ark:%s] prefix cache creation failed (input may be < 1024 tokens): %v", p.config.ID, err)
		return entry.responseID // stale ID (possibly "") — next request will retry
	}

	// Refresh 60 s before actual server expiry to avoid stale-ID windows.
	entry.responseID = info.ResponseID
	entry.expiresAt = time.Now().Add(time.Duration(p.config.PrefixCacheTTL)*time.Second - 60*time.Second)

	logs.CtxInfo(ctx, "[ark:%s] prefix cache created for model %s, response_id=%s, cached_tokens=%d",
		p.config.ID, modelName, info.ResponseID, info.Usage.PromptTokenDetails.CachedTokens)

	return entry.responseID
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
