package provider

import (
	"fmt"
	"strings"
)

type Type string

const (
	OpenAI    Type = "openai"
	Anthropic Type = "anthropic"
	Gemini    Type = "gemini"
	Ollama    Type = "ollama"
	Qwen      Type = "qwen"
)

var SupportedProviders = []Type{
	OpenAI,
	Anthropic,
	Gemini,
	Ollama,
	Qwen,
}

type ModelInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Provider Type   `json:"provider"`
}

type ModelSpec struct {
	ProviderID string
	ModelName  string
}

func (m *ModelSpec) Parse(str string) error {
	parts := strings.SplitN(str, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("invalid model spec format: %s (expected provider_id:model_name)", str)
	}

	m.ProviderID = parts[0]
	m.ModelName = parts[1]
	return nil
}

func ParseModelSpec(str string) (*ModelSpec, error) {
	m := &ModelSpec{}
	if err := m.Parse(str); err != nil {
		return nil, err
	}
	return m, nil
}
