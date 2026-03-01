package openai

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

func (p *Provider) Generate(ctx context.Context, modelName string, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {

	if modelName == "" {
		return nil, fmt.Errorf("model name is required")
	}

	ctx, cancel := context.WithTimeout(ctx, p.config.Timeout)
	defer cancel()

	chatModel, err := p.getOrCreateModel(ctx, modelName)
	if err != nil {
		return nil, fmt.Errorf("failed to get chat model for %s: %w", modelName, err)
	}

	resp, err := chatModel.Generate(ctx, input, opts...)
	if err != nil {
		return nil, fmt.Errorf("openai API call failed: %w", err)
	}

	return resp, nil
}

func (p *Provider) Stream(ctx context.Context, modelName string, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {

	if modelName == "" {
		return nil, fmt.Errorf("model name is required")
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

	chatModel, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		APIKey:  p.config.APIKey,
		Model:   modelName,
		BaseURL: p.config.BaseURL,
		ByAzure: false,
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
