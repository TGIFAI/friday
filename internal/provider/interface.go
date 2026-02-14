package provider

import (
	"context"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type Provider interface {
	// ID returns the configured provider instance identifier.
	// The value is used as the lookup key in the provider registry.
	ID() string

	// Type returns the backend family of this provider instance
	// (for example openai, anthropic, gemini, ollama, or qwen).
	Type() Type

	// IsAvailable reports whether the provider is currently healthy for inference requests.
	// Implementations typically perform a lightweight remote check.
	IsAvailable() bool

	// Close releases provider-owned resources such as background workers and clients.
	// It should be safe to call during shutdown.
	Close() error

	// ListModels returns model metadata currently available from the remote backend.
	// The result is used for health checks and model discovery.
	ListModels(ctx context.Context) ([]ModelInfo, error)

	// Generate performs a single non-streaming chat completion request.
	// modelName is the target backend model identifier. If modelName is empty,
	// implementations may fall back to their configured default model.
	// input contains the full prompt messages for this turn, and opts are forwarded
	// to the underlying eino model call.
	Generate(context.Context, string, []*schema.Message, ...model.Option) (*schema.Message, error)

	// Stream performs a streaming chat completion request.
	// The parameters follow the same contract as Generate.
	Stream(context.Context, string, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error)
}
