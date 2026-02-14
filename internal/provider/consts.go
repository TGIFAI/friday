package provider

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
