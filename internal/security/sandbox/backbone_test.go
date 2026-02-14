package sandbox

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestNewExecutorForToolDisabledSandbox(t *testing.T) {
	exec, enabled, err := NewExecutorForTool("", SandboxConfig{}, "exec")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if enabled {
		t.Fatalf("expected sandbox disabled")
	}
	if exec != nil {
		t.Fatalf("expected nil executor when sandbox disabled")
	}
}

func TestNewExecutorForToolLocalBackbone(t *testing.T) {
	workspace := t.TempDir()
	exec, enabled, err := NewExecutorForTool(workspace, SandboxConfig{
		Enable:       true,
		Runtime:      "local",
		ApplyToTools: []string{"exec"},
	}, "exec")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !enabled {
		t.Fatalf("expected sandbox enabled")
	}

	res, err := exec.Execute(context.Background(), &ExecRequest{
		WorkingDir: workspace,
		Timeout:    2 * time.Second,
		Command: Command{
			Display:  "echo local-backbone",
			UseShell: true,
		},
	})
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", res.ExitCode)
	}
	if !strings.Contains(string(res.Stdout), "local-backbone") {
		t.Fatalf("unexpected stdout: %q", string(res.Stdout))
	}
}

func TestNewExecutorForToolUnsupportedBackbone(t *testing.T) {
	_, _, err := NewExecutorForTool("", SandboxConfig{
		Enable:       true,
		Runtime:      "unknown",
		ApplyToTools: []string{"exec"},
	}, "exec")
	if err == nil {
		t.Fatalf("expected error for unsupported backbone")
	}
	if !strings.Contains(err.Error(), "unsupported sandbox backbone") {
		t.Fatalf("unexpected error: %v", err)
	}
}
