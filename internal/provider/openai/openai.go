package openai

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/tgifai/friday/internal/provider"
)

var _ provider.Provider = (*Provider)(nil)

type Provider struct {
	config   Config
	httpCli  *http.Client
	modelMap map[string]*openai.ChatModel

	isAvailable     bool
	availableModels []provider.ModelInfo

	closeCh chan struct{}

	mu sync.RWMutex
}

func NewProvider(ctx context.Context, config Config) (*Provider, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	p := &Provider{
		config:          config,
		modelMap:        make(map[string]*openai.ChatModel, 4),
		availableModels: make([]provider.ModelInfo, 0),
		isAvailable:     false,
		closeCh:         make(chan struct{}),
		httpCli: &http.Client{
			Timeout:   config.Timeout,
			Transport: &http.Transport{ForceAttemptHTTP2: true},
		},
	}

	p.checkAvailability(ctx)

	go p.startHeartbeat()

	return p, nil
}

func (p *Provider) ID() string {
	return p.config.ID
}

func (p *Provider) Type() provider.Type {
	return provider.OpenAI
}

func (p *Provider) GetAvailableModels() []provider.ModelInfo {
	p.mu.RLock()
	defer p.mu.RUnlock()

	result := make([]provider.ModelInfo, len(p.availableModels))
	copy(result, p.availableModels)
	return result
}

func (p *Provider) IsAvailable() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.isAvailable
}

func (p *Provider) Close() error {

	close(p.closeCh)
	return nil
}

type ListModelsResponse struct {
	Data []struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		Created int64  `json:"created"`
		OwnedBy string `json:"owned_by"`
	} `json:"data"`
	Object string `json:"object"`
}

func (p *Provider) ListModels(ctx context.Context) ([]provider.ModelInfo, error) {

	ctx, cancel := context.WithTimeout(ctx, p.config.Timeout)
	defer cancel()

	url := fmt.Sprintf("%s/models", p.config.BaseURL)

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
	for _, model := range modelsResp.Data {
		modelInfo := provider.ModelInfo{
			ID:       model.ID,
			Name:     model.ID,
			Provider: provider.OpenAI,
		}
		result = append(result, modelInfo)
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("no models returned from API")
	}

	return result, nil
}
