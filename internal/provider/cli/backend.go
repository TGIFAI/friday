package cli

import "context"

// Backend abstracts a CLI-based LLM tool (e.g. Claude Code, Codex).
// The provider invokes Run with a prompt; the CLI handles its own agent loop
// internally and returns the final text response.
type Backend interface {
	// Run executes a prompt and returns the text response plus an optional
	// CLI session ID. cliSessionID is empty on the first call; pass the
	// previously returned value to resume an existing session.
	Run(ctx context.Context, cliSessionID string, prompt string) (response string, newSessionID string, err error)

	// Available reports whether the CLI binary is reachable (in PATH).
	Available() bool

	// Name returns the backend identifier (e.g. "claude-code", "codex").
	Name() string
}
