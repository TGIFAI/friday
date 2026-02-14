package ollama

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	ollamamodel "github.com/cloudwego/eino-ext/components/model/ollama"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	ollamaapi "github.com/eino-contrib/ollama/api"

	"github.com/tgifai/friday/internal/provider"
)

var _ provider.Provider = (*Provider)(nil)

type Provider struct {
	config    Config
	modelMap  map[string]*ollamamodel.ChatModel
	httpCli   *http.Client
	modelsCli *ollamaapi.Client
	mu        sync.RWMutex
}

func NewProvider(_ context.Context, id string, cfgMap map[string]any) (*Provider, error) {
	cfg, err := ParseConfig(id, cfgMap)
	if err != nil {
		return nil, fmt.Errorf("parse ollama config: %w", err)
	}

	baseURL, err := url.Parse(cfg.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base url: %w", err)
	}

	httpCli := &http.Client{Timeout: cfg.Timeout}
	return &Provider{
		config:    *cfg,
		modelMap:  make(map[string]*ollamamodel.ChatModel, 4),
		httpCli:   httpCli,
		modelsCli: ollamaapi.NewClient(baseURL, httpCli),
	}, nil
}

func (p *Provider) ID() string {
	return p.config.ID
}

func (p *Provider) Type() provider.Type {
	return provider.Ollama
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

	lr, err := p.modelsCli.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list ollama models failed: %w", err)
	}

	result := make([]provider.ModelInfo, 0, len(lr.Models))
	for _, modelItem := range lr.Models {
		id := strings.TrimSpace(modelItem.Model)
		if id == "" {
			id = strings.TrimSpace(modelItem.Name)
		}
		if id == "" {
			continue
		}

		result = append(result, provider.ModelInfo{
			ID:       id,
			Name:     id,
			Provider: provider.Ollama,
		})
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("no models returned from ollama API")
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
		return nil, fmt.Errorf("ollama API call failed: %w", err)
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

func (p *Provider) getOrCreateModel(ctx context.Context, modelName string) (*ollamamodel.ChatModel, error) {
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

	chatModel, err := ollamamodel.NewChatModel(ctx, &ollamamodel.ChatModelConfig{
		BaseURL: p.config.BaseURL,
		Timeout: p.config.Timeout,
		Model:   modelName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create chat model for %s: %w", modelName, err)
	}
	p.modelMap[modelName] = chatModel
	return chatModel, nil
}
