package cli

import (
	"context"
	"os/exec"

	"github.com/bytedance/sonic"
)

// ClaudeCode implements Backend for the Claude Code CLI (the `claude` binary).
type ClaudeCode struct {
	model   string
	workDir string
}

func (b *ClaudeCode) Run(ctx context.Context, opts RunOpts, prompt string) (string, error) {
	args := []string{
		"-p", prompt,
		"--output-format", "json",
		"--dangerously-skip-permissions", // non-interactive subprocess, must auto-approve tools
	}

	if opts.SystemPrompt != "" {
		args = append(args, "--system-prompt", opts.SystemPrompt)
	}
	if opts.SessionID != "" {
		args = append(args, "--resume", opts.SessionID)
	}
	if b.model != "" {
		args = append(args, "--model", b.model)
	}

	cmd := exec.CommandContext(ctx, "claude", args...)
	if b.workDir != "" {
		cmd.Dir = b.workDir
	}

	output, err := cmd.Output()
	if err != nil {
		return "", fmtExecError("claude", err)
	}

	// Parse JSON output: {"result": "...", "session_id": "..."}
	var result struct {
		Result    string `json:"result"`
		SessionID string `json:"session_id"`
	}
	if err := sonic.Unmarshal(output, &result); err != nil {
		// Fallback: treat entire output as plain text.
		return string(output), nil
	}
	return result.Result, nil
}

func (b *ClaudeCode) Available() bool {
	_, err := exec.LookPath("claude")
	return err == nil
}

func (b *ClaudeCode) Name() string { return "claude-code" }
