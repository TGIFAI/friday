package cli

import (
	"context"
	"os/exec"
	"strings"
)

// Codex implements Backend for the OpenAI Codex CLI (the `codex` binary).
// Codex has no session reuse — every call receives the full prompt.
type Codex struct {
	model   string
	workDir string
}

func (b *Codex) Run(ctx context.Context, opts RunOpts, prompt string) (string, error) {
	// Codex has no native system-prompt flag; prepend to the user prompt.
	if opts.SystemPrompt != "" {
		prompt = "[System]\n" + opts.SystemPrompt + "\n\n" + prompt
	}

	args := []string{
		"-q", prompt,
		"--full-auto", // non-interactive subprocess, must auto-approve tools
	}
	if b.model != "" {
		args = append(args, "--model", b.model)
	}

	cmd := exec.CommandContext(ctx, "codex", args...)
	if b.workDir != "" {
		cmd.Dir = b.workDir
	}

	output, err := cmd.Output()
	if err != nil {
		return "", fmtExecError("codex", err)
	}
	return strings.TrimSpace(string(output)), nil
}

func (b *Codex) Available() bool {
	_, err := exec.LookPath("codex")
	return err == nil
}

func (b *Codex) Name() string { return "codex" }
