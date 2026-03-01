package agentx

import (
	"context"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"

	"github.com/tgifai/friday/internal/pkg/iobuf"
)

// Backend abstracts CLI differences between Claude Code and Codex.
type Backend interface {
	Name() string
	Available() bool
	Run(ctx context.Context, req *RunRequest) (*RunResult, error)
	Start(ctx context.Context, req *RunRequest) (*Process, error)
	// ParseOutput parses raw CLI stdout into a structured RunResult.
	// Used by async completion goroutines to extract session IDs and results.
	ParseOutput(raw string, exitCode int) *RunResult
}

// RunRequest holds parameters for a CLI agent invocation.
type RunRequest struct {
	Prompt       string
	WorkingDir   string
	SystemPrompt string
	MaxTurns     int
	ResumeID     string // CLI-native session ID for --resume
}

// RunResult holds the output of a completed CLI invocation.
type RunResult struct {
	CLISessionID string
	Output       string
	ExitCode     int
}

// Process represents a running async CLI invocation.
type Process struct {
	cmd    *exec.Cmd
	stdout *iobuf.LimitedBuffer
	stderr *iobuf.LimitedBuffer
	done   chan struct{}

	mu       sync.RWMutex
	exitCode int
	waitErr  string
	finished bool
}

// Done returns a channel that closes when the process exits.
// Returns nil if the process was constructed without a done channel
// (e.g. in tests or mocks).
func (p *Process) Done() <-chan struct{} { return p.done }

// Result collects the process output after it finishes.
// Safe to call on a zero-value Process (nil stdout).
func (p *Process) Result() *RunResult {
	p.mu.RLock()
	defer p.mu.RUnlock()
	var output string
	if p.stdout != nil {
		output = p.stdout.String()
	}
	return &RunResult{
		Output:   output,
		ExitCode: p.exitCode,
	}
}

// Kill terminates the process.
// Safe to call when cmd or cmd.Process is nil.
func (p *Process) Kill() {
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Signal(os.Kill)
	}
}

// seq is a package-level counter for generating unique session IDs.
var seq atomic.Int64
