package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
		backend = &ClaudeCode{workDir: cfg.WorkDir}
	case "codex":
		backend = &Codex{workDir: cfg.WorkDir}
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
	name := p.backend.Name()
	return []provider.ModelInfo{{
		ID:       name,
		Name:     name,
		Provider: provider.CLI,
	}}, nil
}

func (p *Provider) RegisterTools(_ []*schema.ToolInfo) {}

func (p *Provider) Generate(ctx context.Context, modelName string, input []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	if modelName == "" {
		return nil, fmt.Errorf("model name is required")
	}

	// Apply timeout.
	ctx, cancel := context.WithTimeout(ctx, p.config.Timeout)
	defer cancel()

	// Derive a deterministic CLI session ID from the friday session key.
	// This ties the CLI session to the friday session so that resume
	// works across process restarts without storing extra metadata.
	// SessionID is only set for resume (existing messages); on the first
	// call the backend creates a fresh session.
	sess := session.ExtractFromCtx(ctx)
	var opts RunOpts
	resume := false
	if sess != nil && sess.SessionKey != "" {
		resume = sess.MsgCount() > 0
		if resume {
			opts.SessionID = hashSessionKey(sess.SessionKey)
		}
	}

	// Split system messages from conversation messages.
	var prompt string
	opts.SystemPrompt, prompt = buildPrompt(input, resume)

	// Run the CLI backend.
	response, err := p.backend.Run(ctx, opts, prompt)
	if err != nil {
		return nil, fmt.Errorf("cli backend %s: %w", p.backend.Name(), err)
	}

	return &schema.Message{Role: schema.Assistant, Content: response}, nil
}

func (p *Provider) Stream(_ context.Context, _ string, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, fmt.Errorf("cli provider does not support streaming")
}

// hashSessionKey produces a short hex string from a friday session key,
// suitable as a deterministic CLI session identifier.
func hashSessionKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:16]) // 32 hex chars
}

// fmtExecError wraps an exec error, including stderr when available.
func fmtExecError(bin string, err error) error {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
		return fmt.Errorf("%s cli (exit %d): %s", bin, exitErr.ExitCode(), strings.TrimSpace(string(exitErr.Stderr)))
	}
	return fmt.Errorf("%s cli: %w", bin, err)
}

// buildPrompt converts a slice of messages into a system prompt and a
// conversation prompt. System messages are returned separately so that
// backends with native system-prompt support (e.g. Claude Code) can pass
// them through the dedicated flag.
// In resume mode (hasSession=true) only the latest user message is sent,
// since the CLI already has the prior context from its own session.
func buildPrompt(msgs []*schema.Message, hasSession bool) (systemPrompt string, prompt string) {
	if len(msgs) == 0 {
		return "", ""
	}

	// Collect system messages.
	var sys strings.Builder
	for _, m := range msgs {
		if m.Role == schema.System {
			if sys.Len() > 0 {
				sys.WriteString("\n\n")
			}
			sys.WriteString(m.Content)
		}
	}
	systemPrompt = strings.TrimSpace(sys.String())

	if hasSession {
		// Only send the latest user message.
		for i := len(msgs) - 1; i >= 0; i-- {
			if msgs[i].Role == schema.User {
				return systemPrompt, msgs[i].Content
			}
		}
		return systemPrompt, msgs[len(msgs)-1].Content
	}

	// Full mode: render non-system messages as text.
	var b strings.Builder
	for _, m := range msgs {
		switch m.Role {
		case schema.System:
			// Already extracted above.
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
	return systemPrompt, strings.TrimSpace(b.String())
}
