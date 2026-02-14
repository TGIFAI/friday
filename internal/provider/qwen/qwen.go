package qwen

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/bytedance/sonic"
	qwenmodel "github.com/cloudwego/eino-ext/components/model/qwen"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/tgifai/friday/internal/provider"
)

var _ provider.Provider = (*Provider)(nil)

type Provider struct {
	config   Config
	httpCli  *http.Client
	modelMap map[string]*qwenmodel.ChatModel
	mu       sync.RWMutex
}

func NewProvider(config Config) (*Provider, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &Provider{
		config:   config,
		httpCli:  &http.Client{Timeout: config.Timeout},
		modelMap: make(map[string]*qwenmodel.ChatModel, 4),
	}, nil
}

func (p *Provider) ID() string {
	return p.config.ID
}

func (p *Provider) Type() provider.Type {
	return provider.Qwen
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

type ListModelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

func (p *Provider) ListModels(ctx context.Context) ([]provider.ModelInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, p.config.Timeout)
	defer cancel()

	url := fmt.Sprintf("%s/models", strings.TrimRight(p.config.BaseURL, "/"))
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", p.config.APIKey))
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpCli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to list models from API: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var modelsResp ListModelsResponse
	if err := sonic.Unmarshal(body, &modelsResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	result := make([]provider.ModelInfo, 0, len(modelsResp.Data))
	for _, modelItem := range modelsResp.Data {
		id := strings.TrimSpace(modelItem.ID)
		if id == "" {
			continue
		}
		result = append(result, provider.ModelInfo{
			ID:       id,
			Name:     id,
			Provider: provider.Qwen,
		})
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("no models returned from API")
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
		return nil, fmt.Errorf("qwen API call failed: %w", err)
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

func (p *Provider) getOrCreateModel(ctx context.Context, modelName string) (*qwenmodel.ChatModel, error) {
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

	chatModel, err := qwenmodel.NewChatModel(ctx, &qwenmodel.ChatModelConfig{
		APIKey:  p.config.APIKey,
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
