package shellx

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestParseCommandArgSupportsStringAndSlice(t *testing.T) {
	t.Run("string", func(t *testing.T) {
		cmd, err := parseCommandArg("echo hello")
		if err != nil {
			t.Fatalf("parseCommandArg(string) error: %v", err)
		}
		if cmd == nil || !cmd.useShell || cmd.display != "echo hello" {
			t.Fatalf("unexpected parsed command: %+v", cmd)
		}
	})

	t.Run("slice", func(t *testing.T) {
		cmd, err := parseCommandArg([]interface{}{"echo", "hello"})
		if err != nil {
			t.Fatalf("parseCommandArg([]interface{}) error: %v", err)
		}
		if cmd == nil || cmd.useShell || cmd.program != "echo" {
			t.Fatalf("unexpected parsed command: %+v", cmd)
		}
		if len(cmd.argv) != 1 || cmd.argv[0] != "hello" {
			t.Fatalf("unexpected argv: %+v", cmd.argv)
		}
	})
}

func TestExecToolExecuteSupportsCommandArray(t *testing.T) {
	echoPath, err := exec.LookPath("echo")
	if err != nil {
		t.Fatalf("echo not found in PATH: %v", err)
	}

	tl := NewExecTool("")
	out, err := tl.Execute(context.Background(), map[string]interface{}{
		"command": []interface{}{echoPath, "hello-from-array"},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	res, ok := out.(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected output type: %T", out)
	}
	if !res["success"].(bool) {
		t.Fatalf("expected success=true, got %+v", res)
	}
	stdout := res["stdout"].(string)
	if !strings.Contains(stdout, "hello-from-array") {
		t.Fatalf("unexpected stdout: %q", stdout)
	}
}

func TestExecToolTimeoutReturnsError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sleep command test is unix-focused")
	}

	tl := NewExecTool("")
	_, err := tl.Execute(context.Background(), map[string]interface{}{
		"command": "sleep 2",
		"timeout": 0.1,
	})
	if err == nil {
		t.Fatalf("expected timeout error")
	}
	if !strings.Contains(err.Error(), "command timeout") {
		t.Fatalf("expected timeout error, got: %v", err)
	}
}

func TestExecToolReturnsStdoutStderrAndExitCode(t *testing.T) {
	tl := NewExecTool("")
	out, err := tl.Execute(context.Background(), map[string]interface{}{
		"command": "echo out; echo err >&2; exit 7",
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	res, ok := out.(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected output type: %T", out)
	}
	if res["success"].(bool) {
		t.Fatalf("expected success=false, got %+v", res)
	}
	if res["exit_code"].(int) != 7 {
		t.Fatalf("expected exit code 7, got %+v", res["exit_code"])
	}
	if !strings.Contains(res["stdout"].(string), "out") {
		t.Fatalf("unexpected stdout: %q", res["stdout"])
	}
	if !strings.Contains(res["stderr"].(string), "err") {
		t.Fatalf("unexpected stderr: %q", res["stderr"])
	}
}

func TestProcessToolLifecycle(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process lifecycle test is unix-focused")
	}

	tl := NewProcessTool("")

	startOut, err := tl.Execute(context.Background(), map[string]interface{}{
		"action":  "start",
		"command": "echo started; sleep 1",
	})
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}
	startRes := startOut.(map[string]interface{})
	processID := startRes["process_id"].(string)
	if processID == "" {
		t.Fatalf("empty process_id")
	}

	deadline := time.Now().Add(4 * time.Second)
	var status map[string]interface{}
	for {
		statusOut, err := tl.Execute(context.Background(), map[string]interface{}{
			"action":     "status",
			"process_id": processID,
		})
		if err != nil {
			t.Fatalf("status failed: %v", err)
		}
		status = statusOut.(map[string]interface{})
		if running, _ := status["running"].(bool); !running {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("process did not finish in time, status=%+v", status)
		}
		time.Sleep(50 * time.Millisecond)
	}

	logOut, err := tl.Execute(context.Background(), map[string]interface{}{
		"action":     "log",
		"process_id": processID,
	})
	if err != nil {
		t.Fatalf("log failed: %v", err)
	}
	logRes := logOut.(map[string]interface{})
	if !strings.Contains(logRes["stdout"].(string), "started") {
		t.Fatalf("unexpected process stdout: %q", logRes["stdout"])
	}

	listOut, err := tl.Execute(context.Background(), map[string]interface{}{
		"action": "list",
	})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	listRes := listOut.([]map[string]interface{})
	found := false
	for _, item := range listRes {
		if item["process_id"] == processID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("process %s not found in list", processID)
	}
}

func TestExecToolTimeoutCappedAtMax(t *testing.T) {
	tl := NewExecTool("")
	timeout := tl.resolveTimeout(map[string]interface{}{"timeout": 9999})
	if timeout != maxTimeout {
		t.Fatalf("expected timeout capped at %v, got %v", maxTimeout, timeout)
	}
}

func TestExecToolOutputTruncation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-focused")
	}

	tl := NewExecTool("")
	// Generate output larger than maxExecOutputBytes (1 MiB).
	// Each line is ~80 chars, so 20000 lines ≈ 1.6 MiB.
	out, err := tl.Execute(context.Background(), map[string]interface{}{
		"command": "yes 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa' | head -n 20000",
		"timeout": 10,
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	res := out.(map[string]interface{})
	if res["truncated"] != true {
		t.Fatalf("expected truncated=true for large output")
	}
	stdout := res["stdout"].(string)
	if len(stdout) > maxExecOutputBytes {
		t.Fatalf("stdout should be capped at %d bytes, got %d", maxExecOutputBytes, len(stdout))
	}
}

func TestResolveWorkDir(t *testing.T) {
	workspace := t.TempDir()

	t.Run("empty returns workspace", func(t *testing.T) {
		got := resolveWorkDir(workspace, map[string]interface{}{})
		if got != workspace {
			t.Fatalf("expected %q, got %q", workspace, got)
		}
	})

	t.Run("relative path joined with workspace", func(t *testing.T) {
		got := resolveWorkDir(workspace, map[string]interface{}{"working_dir": "sub"})
		want := filepath.Join(workspace, "sub")
		if got != want {
			t.Fatalf("expected %q, got %q", want, got)
		}
	})

	t.Run("absolute path outside workspace rejected", func(t *testing.T) {
		got := resolveWorkDir(workspace, map[string]interface{}{"working_dir": "/tmp"})
		if got != workspace {
			t.Fatalf("expected workspace %q for out-of-scope path, got %q", workspace, got)
		}
	})

	t.Run("absolute path inside workspace allowed", func(t *testing.T) {
		sub := filepath.Join(workspace, "inner")
		got := resolveWorkDir(workspace, map[string]interface{}{"working_dir": sub})
		if got != sub {
			t.Fatalf("expected %q, got %q", sub, got)
		}
	})
}

func TestProcessToolActiveLimit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-focused")
	}

	tl := NewProcessTool("")

	// Start maxActiveProcesses long-running processes.
	for i := 0; i < maxActiveProcesses; i++ {
		_, err := tl.Execute(context.Background(), map[string]interface{}{
			"action":  "start",
			"command": "sleep 30",
		})
		if err != nil {
			t.Fatalf("start #%d failed: %v", i, err)
		}
	}

	// The next start should be rejected.
	_, err := tl.Execute(context.Background(), map[string]interface{}{
		"action":  "start",
		"command": "echo should-fail",
	})
	if err == nil {
		t.Fatal("expected error when exceeding active process limit")
	}
	if !strings.Contains(err.Error(), "too many active processes") {
		t.Fatalf("unexpected error: %v", err)
	}

	// Kill all.
	for i := 1; i <= maxActiveProcesses; i++ {
		_, _ = tl.Execute(context.Background(), map[string]interface{}{
			"action":     "kill",
			"process_id": fmt.Sprintf("proc-%d", i),
		})
	}
}

func TestProcessToolKill(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process kill test is unix-focused")
	}

	tl := NewProcessTool("")
	startOut, err := tl.Execute(context.Background(), map[string]interface{}{
		"action":  "start",
		"command": "sleep 5",
	})
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}
	processID := startOut.(map[string]interface{})["process_id"].(string)

	_, err = tl.Execute(context.Background(), map[string]interface{}{
		"action":     "kill",
		"process_id": processID,
	})
	if err != nil {
		t.Fatalf("kill failed: %v", err)
	}

	deadline := time.Now().Add(4 * time.Second)
	for {
		statusOut, err := tl.Execute(context.Background(), map[string]interface{}{
			"action":     "status",
			"process_id": processID,
		})
		if err != nil {
			t.Fatalf("status failed: %v", err)
		}
		status := statusOut.(map[string]interface{})
		if running, _ := status["running"].(bool); !running {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("process still running after kill, status=%+v", status)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
