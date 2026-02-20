package anthropic

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/cloudwego/eino-ext/components/model/claude"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/tgifai/friday/internal/provider"
)

var _ provider.Provider = (*Provider)(nil)

type Provider struct {
	config   Config
	httpCli  *http.Client
	modelMap map[string]*claude.ChatModel
	mu       sync.RWMutex
}

func NewProvider(_ context.Context, id string, cfgMap map[string]any) (*Provider, error) {
	cfg, err := ParseConfig(id, cfgMap)
	if err != nil {
		return nil, fmt.Errorf("parse anthropic config: %w", err)
	}

	return &Provider{
		config:   *cfg,
		httpCli:  &http.Client{Timeout: cfg.Timeout},
		modelMap: make(map[string]*claude.ChatModel, 4),
	}, nil
}

func (p *Provider) ID() string {
	return p.config.ID
}

func (p *Provider) Type() provider.Type {
	return provider.Anthropic
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

type listModelsResponse struct {
	Data []struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
	} `json:"data"`
}

func (p *Provider) ListModels(ctx context.Context) ([]provider.ModelInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, p.config.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.modelsEndpoint(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("x-api-key", p.config.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")

	resp, err := p.httpCli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to list models from API: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var modelsResp listModelsResponse
	if err := sonic.Unmarshal(body, &modelsResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	result := make([]provider.ModelInfo, 0, len(modelsResp.Data))
	for _, item := range modelsResp.Data {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		name := strings.TrimSpace(item.DisplayName)
		if name == "" {
			name = id
		}
		result = append(result, provider.ModelInfo{
			ID:       id,
			Name:     name,
			Provider: provider.Anthropic,
		})
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("no models returned from API")
	}

	return result, nil
}

func (p *Provider) Generate(ctx context.Context, modelName string, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	if modelName == "" {
		modelName = p.config.DefaultModel
	}

	ctx, cancel := context.WithTimeout(ctx, p.config.Timeout)
	defer cancel()

	chatModel, err := p.getOrCreateModel(ctx, modelName)
	if err != nil {
		return nil, fmt.Errorf("failed to get chat model for %s: %w", modelName, err)
	}

	sanitizeMessages(messages)

	resp, err := chatModel.Generate(ctx, messages, opts...)
	if err != nil {
		return nil, fmt.Errorf("anthropic API call failed: %w", err)
	}

	return resp, nil
}

func (p *Provider) Stream(ctx context.Context, modelName string, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	if modelName == "" {
		modelName = p.config.DefaultModel
	}

	ctx, cancel := context.WithTimeout(ctx, p.config.Timeout)
	defer cancel()

	chatModel, err := p.getOrCreateModel(ctx, modelName)
	if err != nil {
		return nil, fmt.Errorf("failed to get chat model for %s: %w", modelName, err)
	}

	sanitizeMessages(messages)

	streamReader, err := chatModel.Stream(ctx, messages, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create stream: %w", err)
	}

	return streamReader, nil
}

// sanitizeMessages ensures no message has completely empty content, which would
// cause a panic in the upstream eino-ext Claude SDK (index out of range [-1]
// in populateInput when MessageParam.Content is empty).
func sanitizeMessages(msgs []*schema.Message) {
	for _, m := range msgs {
		if m.Content != "" || len(m.ToolCalls) > 0 ||
			len(m.UserInputMultiContent) > 0 ||
			len(m.AssistantGenMultiContent) > 0 ||
			len(m.MultiContent) > 0 {
			continue
		}
		// Message has no content at all — fill in a placeholder to prevent
		// the SDK from producing an empty Content slice.
		if m.Role == schema.Tool {
			m.Content = "{}"
		} else {
			m.Content = "..."
		}
	}
}

func (p *Provider) getOrCreateModel(ctx context.Context, modelName string) (*claude.ChatModel, error) {
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

	baseURL := strings.TrimSpace(p.config.BaseURL)
	var baseURLPtr *string
	if baseURL != "" {
		baseURLPtr = &baseURL
	}

	chatModel, err := claude.NewChatModel(ctx, &claude.Config{
		APIKey:    p.config.APIKey,
		BaseURL:   baseURLPtr,
		Model:     modelName,
		MaxTokens: p.config.MaxTokens,
		HTTPClient: &http.Client{
			Timeout: p.config.Timeout,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create chat model for %s: %w", modelName, err)
	}

	p.modelMap[modelName] = chatModel
	return chatModel, nil
}

func (p *Provider) modelsEndpoint() string {
	base := strings.TrimRight(strings.TrimSpace(p.config.BaseURL), "/")
	if strings.HasSuffix(base, "/v1") {
		return base + "/models"
	}
	return base + "/v1/models"
}
