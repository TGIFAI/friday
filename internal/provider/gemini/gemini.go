package gemini

import (
	"context"
	"fmt"
	"strings"
	"sync"

	gmodel "github.com/cloudwego/eino-ext/components/model/gemini"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"google.golang.org/genai"

	"github.com/tgifai/friday/internal/provider"
)

var _ provider.Provider = (*Provider)(nil)

type Provider struct {
	config   Config
	client   *genai.Client
	modelMap map[string]model.ToolCallingChatModel
	tools    []*schema.ToolInfo
	mu       sync.RWMutex
}

func (p *Provider) RegisterTools(tools []*schema.ToolInfo) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.tools = tools
}

func NewProvider(ctx context.Context, id string, cfgMap map[string]any) (*Provider, error) {
	cfg, err := ParseConfig(id, cfgMap)
	if err != nil {
		return nil, fmt.Errorf("parse gemini config: %w", err)
	}

	clientCfg := &genai.ClientConfig{
		APIKey: cfg.APIKey,
	}
	if cfg.BaseURL != "" {
		clientCfg.HTTPOptions = genai.HTTPOptions{BaseURL: cfg.BaseURL}
	}

	client, err := genai.NewClient(ctx, clientCfg)
	if err != nil {
		return nil, fmt.Errorf("new gemini client failed: %w", err)
	}

	return &Provider{
		config:   *cfg,
		client:   client,
		modelMap: make(map[string]model.ToolCallingChatModel, 4),
	}, nil
}

func (p *Provider) ID() string {
	return p.config.ID
}

func (p *Provider) Type() provider.Type {
	return provider.Gemini
}

func (p *Provider) IsAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), p.config.Timeout)
	defer cancel()
	_, err := p.ListModels(ctx)
	return err == nil
}

func (p *Provider) Close() error {
	return nil
}

func (p *Provider) ListModels(ctx context.Context) ([]provider.ModelInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, p.config.Timeout)
	defer cancel()

	result := make([]provider.ModelInfo, 0, 16)
	for item, err := range p.client.Models.All(ctx) {
		if err != nil {
			return nil, fmt.Errorf("list gemini models failed: %w", err)
		}
		if item == nil {
			continue
		}
		id := strings.TrimSpace(item.Name)
		if id == "" {
			continue
		}
		id = strings.TrimPrefix(id, "models/")
		name := strings.TrimSpace(item.DisplayName)
		if name == "" {
			name = id
		}
		result = append(result, provider.ModelInfo{
			ID:       id,
			Name:     name,
			Provider: provider.Gemini,
		})
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("no models returned from gemini API")
	}

	return result, nil
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
	resp, err := chatModel.Generate(ctx, input, opts...)
	if err != nil {
		return nil, fmt.Errorf("gemini API call failed: %w", err)
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
	streamReader, err := chatModel.Stream(ctx, input, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create stream: %w", err)
	}
	return streamReader, nil
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

	chatModel, err := gmodel.NewChatModel(ctx, &gmodel.Config{
		Client: p.client,
		Model:  modelName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create chat model for %s: %w", modelName, err)
	}

	cm, err := chatModel.WithTools(p.tools)
	if err != nil {
		return nil, fmt.Errorf("failed to bind tools with model for %s: %w", modelName, err)
	}

	p.modelMap[modelName] = cm
	return cm, nil
}
