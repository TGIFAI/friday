package cli

import "context"

// RunOpts holds per-call options for a Backend.Run invocation.
type RunOpts struct {
	// SessionID is the deterministic session identifier for this conversation.
	// Empty on the first call; non-empty to resume an existing session.
	SessionID string

	// SystemPrompt carries system instructions that the backend should pass
	// via its native mechanism (e.g. --system-prompt for Claude Code).
	SystemPrompt string
}

// Backend abstracts a CLI-based LLM tool (e.g. Claude Code, Codex).
// The provider invokes Run with a prompt; the CLI handles its own agent loop
// internally and returns the final text response.
type Backend interface {
	// Run executes a prompt and returns the text response.
	Run(ctx context.Context, opts RunOpts, prompt string) (response string, err error)

	// Available reports whether the CLI binary is reachable (in PATH).
	Available() bool

	// Name returns the backend identifier (e.g. "claude-code", "codex").
	Name() string
}
