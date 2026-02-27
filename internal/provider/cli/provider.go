package cli

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/tgifai/friday/internal/agent/session"

	"github.com/tgifai/friday/internal/provider"
)

var _ provider.Provider = (*Provider)(nil)

type Provider struct {
	config  Config
	backend Backend
	mu      sync.RWMutex
}

func NewProvider(_ context.Context, id string, cfgMap map[string]any) (*Provider, error) {
	cfg, err := ParseConfig(id, cfgMap)
	if err != nil {
		return nil, err
	}

	var backend Backend
	switch cfg.Backend {
	case "claude-code":
		backend = &ClaudeCode{model: cfg.DefaultModel, workDir: cfg.WorkDir}
	case "codex":
		backend = &Codex{model: cfg.DefaultModel, workDir: cfg.WorkDir}
	default:
		return nil, fmt.Errorf("unsupported cli backend: %s", cfg.Backend)
	}

	return &Provider{config: *cfg, backend: backend}, nil
}

func (p *Provider) ID() string          { return p.config.ID }
func (p *Provider) Type() provider.Type { return provider.CLI }
func (p *Provider) IsAvailable() bool   { return p.backend.Available() }
func (p *Provider) Close() error        { return nil }

func (p *Provider) ListModels(_ context.Context) ([]provider.ModelInfo, error) {
	name := p.config.DefaultModel
	if name == "" {
		name = p.backend.Name()
	}
	return []provider.ModelInfo{{
		ID:       name,
		Name:     name,
		Provider: provider.CLI,
	}}, nil
}

func (p *Provider) Generate(ctx context.Context, modelName string, input []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	if modelName == "" {
		modelName = p.config.DefaultModel
	}

	// Apply timeout.
	ctx, cancel := context.WithTimeout(ctx, p.config.Timeout)
	defer cancel()

	// Read stored CLI session ID from context-injected session.
	sess := session.ExtractFromCtx(ctx)
	var cliSessionID string
	if sess != nil {
		cliSessionID = sess.GetMeta("cli:session_id")
	}

	// Build the text prompt from the message list.
	prompt := buildPrompt(input, cliSessionID != "")

	// Run the CLI backend.
	response, newSessionID, err := p.backend.Run(ctx, cliSessionID, prompt)
	if err != nil {
		return nil, fmt.Errorf("cli backend %s: %w", p.backend.Name(), err)
	}

	// Persist CLI session ID for future resume.
	if sess != nil && newSessionID != "" {
		sess.SetMeta("cli:session_id", newSessionID)
	}

	return &schema.Message{Role: schema.Assistant, Content: response}, nil
}

func (p *Provider) Stream(_ context.Context, _ string, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, fmt.Errorf("cli provider does not support streaming")
}

// fmtExecError wraps an exec error, including stderr when available.
func fmtExecError(bin string, err error) error {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
		return fmt.Errorf("%s cli (exit %d): %s", bin, exitErr.ExitCode(), strings.TrimSpace(string(exitErr.Stderr)))
	}
	return fmt.Errorf("%s cli: %w", bin, err)
}

// buildPrompt converts a slice of messages into a single text prompt.
// In resume mode (hasSession=true) only the latest user message is sent,
// since the CLI already has the prior context from its own session.
func buildPrompt(msgs []*schema.Message, hasSession bool) string {
	if len(msgs) == 0 {
		return ""
	}

	if hasSession {
		// Only send the latest user message.
		for i := len(msgs) - 1; i >= 0; i-- {
			if msgs[i].Role == schema.User {
				return msgs[i].Content
			}
		}
		return msgs[len(msgs)-1].Content
	}

	// Full mode: render all messages as text.
	var b strings.Builder
	for _, m := range msgs {
		switch m.Role {
		case schema.System:
			b.WriteString("[System]\n")
			b.WriteString(m.Content)
			b.WriteString("\n\n")
		case schema.User:
			b.WriteString("[User]\n")
			b.WriteString(m.Content)
			b.WriteString("\n\n")
		case schema.Assistant:
			b.WriteString("[Assistant]\n")
			b.WriteString(m.Content)
			b.WriteString("\n\n")
		case schema.Tool:
			// Skip tool messages — the CLI handles tools internally.
		}
	}
	return strings.TrimSpace(b.String())
}
