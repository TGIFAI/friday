package agentx

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/bytedance/sonic"
)

const maxOutputBytes = 1 << 20 // 1 MiB

// ClaudeCodeBackend wraps the claude CLI in non-interactive pipe mode.
type ClaudeCodeBackend struct{}

// Compile-time check that ClaudeCodeBackend implements Backend.
var _ Backend = (*ClaudeCodeBackend)(nil)

func (b *ClaudeCodeBackend) Name() string { return "claude-code" }

func (b *ClaudeCodeBackend) Available() bool {
	_, err := exec.LookPath("claude")
	return err == nil
}

func (b *ClaudeCodeBackend) buildArgs(req *RunRequest) []string {
	args := []string{"-p", req.Prompt, "--dangerously-skip-permissions", "--output-format", "json"}
	if req.ResumeID != "" {
		args = append(args, "--resume", req.ResumeID)
	}
	if req.SystemPrompt != "" {
		args = append(args, "--append-system-prompt", req.SystemPrompt)
	}
	if req.MaxTurns > 0 {
		args = append(args, "--max-turns", strconv.Itoa(req.MaxTurns))
	}
	return args
}

// claudeOutput is the JSON structure emitted by claude --output-format json.
type claudeOutput struct {
	Result    string `json:"result"`
	SessionID string `json:"session_id"`
}

func (b *ClaudeCodeBackend) parseResult(raw string, exitCode int) (*RunResult, error) {
	var out claudeOutput
	if err := sonic.UnmarshalString(raw, &out); err != nil || out.Result == "" {
		// Fallback: treat raw text as output.
		return &RunResult{
			Output:   strings.TrimSpace(raw),
			ExitCode: exitCode,
		}, nil
	}
	return &RunResult{
		CLISessionID: out.SessionID,
		Output:       out.Result,
		ExitCode:     exitCode,
	}, nil
}

func (b *ClaudeCodeBackend) ParseOutput(raw string, exitCode int) *RunResult {
	res, _ := b.parseResult(raw, exitCode)
	return res
}

func (b *ClaudeCodeBackend) Run(ctx context.Context, req *RunRequest) (*RunResult, error) {
	args := b.buildArgs(req)
	cmd := exec.CommandContext(ctx, "claude", args...)
	if req.WorkingDir != "" {
		cmd.Dir = req.WorkingDir
	}

	stdout := newLimitedBuffer(maxOutputBytes)
	stderr := newLimitedBuffer(maxOutputBytes)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	exitCode := 0
	err := cmd.Run()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, fmt.Errorf("claude-code run: %w", err)
		}
	}

	return b.parseResult(stdout.String(), exitCode)
}

func (b *ClaudeCodeBackend) Start(ctx context.Context, req *RunRequest) (*Process, error) {
	args := b.buildArgs(req)
	cmd := exec.CommandContext(ctx, "claude", args...)
	if req.WorkingDir != "" {
		cmd.Dir = req.WorkingDir
	}

	stdout := newLimitedBuffer(maxOutputBytes)
	stderr := newLimitedBuffer(maxOutputBytes)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("claude-code start: %w", err)
	}

	p := &Process{
		cmd:    cmd,
		stdout: stdout,
		stderr: stderr,
		done:   make(chan struct{}),
	}

	go func() {
		defer close(p.done)
		waitErr := cmd.Wait()

		p.mu.Lock()
		defer p.mu.Unlock()
		p.finished = true
		if waitErr != nil {
			var exitErr *exec.ExitError
			if errors.As(waitErr, &exitErr) {
				p.exitCode = exitErr.ExitCode()
			}
			p.waitErr = waitErr.Error()
		}
	}()

	return p, nil
}
