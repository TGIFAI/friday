package agentx

import (
	"context"
	"errors"
	"os/exec"
	"strings"

	"github.com/bytedance/sonic"
)

// CodexBackend wraps the codex CLI in non-interactive mode.
type CodexBackend struct{}

var _ Backend = (*CodexBackend)(nil)

func (b *CodexBackend) Name() string { return "codex" }

func (b *CodexBackend) Available() bool {
	_, err := exec.LookPath("codex")
	return err == nil
}

func (b *CodexBackend) buildArgs(req *RunRequest) []string {
	args := []string{"exec"}
	if req.ResumeID != "" {
		args = append(args, "resume", req.ResumeID)
	}
	args = append(args, req.Prompt)
	args = append(args, "--json", "--dangerously-bypass-approvals-and-sandbox")
	return args
}

// codexEvent represents a single JSONL event emitted by the codex CLI.
type codexEvent struct {
	Type     string    `json:"type"`
	ThreadID string    `json:"thread_id,omitempty"`
	Item     *codexItem `json:"item,omitempty"`
}

// codexItem represents an item within a codex event.
type codexItem struct {
	Type    string         `json:"type"`
	Role    string         `json:"role"`
	Content []codexContent `json:"content"`
}

// codexContent represents a content block inside a codex item.
type codexContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func (b *CodexBackend) parseJSONL(raw string, exitCode int) *RunResult {
	result := &RunResult{
		ExitCode: exitCode,
	}

	var lastAssistantText string
	lines := strings.Split(raw, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var ev codexEvent
		if err := sonic.UnmarshalString(line, &ev); err != nil {
			continue
		}

		// Capture thread_id from any event that has it.
		if ev.ThreadID != "" {
			result.CLISessionID = ev.ThreadID
		}

		// Look for assistant messages and track the last one.
		if ev.Item != nil && ev.Item.Role == "assistant" {
			for _, c := range ev.Item.Content {
				if c.Type == "text" && c.Text != "" {
					lastAssistantText = c.Text
				}
			}
		}
	}

	if lastAssistantText != "" {
		result.Output = lastAssistantText
	} else {
		// Fallback to raw text when no assistant messages found.
		result.Output = raw
	}

	return result
}

func (b *CodexBackend) Run(ctx context.Context, req *RunRequest) (*RunResult, error) {
	args := b.buildArgs(req)
	cmd := exec.CommandContext(ctx, "codex", args...)
	if req.WorkingDir != "" {
		cmd.Dir = req.WorkingDir
	}

	stdout := newLimitedBuffer(maxOutputBytes)
	stderr := newLimitedBuffer(maxOutputBytes)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, err
		}
	}

	result := b.parseJSONL(stdout.String(), exitCode)
	return result, nil
}

func (b *CodexBackend) Start(ctx context.Context, req *RunRequest) (*Process, error) {
	args := b.buildArgs(req)
	cmd := exec.CommandContext(ctx, "codex", args...)
	if req.WorkingDir != "" {
		cmd.Dir = req.WorkingDir
	}

	stdoutBuf := newLimitedBuffer(maxOutputBytes)
	stderrBuf := newLimitedBuffer(maxOutputBytes)
	cmd.Stdout = stdoutBuf
	cmd.Stderr = stderrBuf

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	done := make(chan struct{})
	p := &Process{
		cmd:    cmd,
		stdout: stdoutBuf,
		stderr: stderrBuf,
		done:   done,
	}

	go func() {
		defer close(done)
		waitErr := cmd.Wait()
		p.mu.Lock()
		defer p.mu.Unlock()
		p.finished = true
		if waitErr != nil {
			var exitErr *exec.ExitError
			if errors.As(waitErr, &exitErr) {
				p.exitCode = exitErr.ExitCode()
			} else {
				p.waitErr = waitErr.Error()
				p.exitCode = -1
			}
		}
	}()

	return p, nil
}
