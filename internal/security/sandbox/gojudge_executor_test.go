package sandbox

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bytedance/sonic"
)

func TestGoJudgeExecutorExecuteSuccess(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "sub"), 0o755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/run" {
			http.Error(w, "unexpected path", http.StatusNotFound)
			return
		}

		var payload map[string]interface{}
		if err := sonic.ConfigDefault.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		cmds, _ := payload["cmd"].([]interface{})
		if len(cmds) != 1 {
			http.Error(w, "missing cmd", http.StatusBadRequest)
			return
		}
		cmd, _ := cmds[0].(map[string]interface{})
		cwd, _ := cmd["cwd"].(string)
		if cwd != "/w/sub" {
			http.Error(w, "unexpected cwd: "+cwd, http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"status":"Accepted","exitStatus":0,"files":{"stdout":"ok\n","stderr":""}}]}`))
	}))
	defer srv.Close()

	exec := NewGoJudgeExecutor(workspace, GoJudgeConfig{
		Endpoint:          srv.URL,
		RequestTimeoutSec: 10,
		WorkdirMount:      "/w",
		CPULimitMS:        4000,
		WallLimitMS:       10000,
		MemoryLimitKB:     262144,
		ProcLimit:         128,
		MaxStdoutBytes:    1024,
		MaxStderrBytes:    1024,
	})

	res, err := exec.Execute(context.Background(), &ExecRequest{
		WorkingDir: filepath.Join(workspace, "sub"),
		Timeout:    4 * time.Second,
		Command: Command{
			Display:  "echo hello",
			UseShell: true,
		},
	})
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", res.ExitCode)
	}
	if string(res.Stdout) != "ok\n" {
		t.Fatalf("unexpected stdout: %q", string(res.Stdout))
	}
}

func TestGoJudgeExecutorTimeoutStatus(t *testing.T) {
	workspace := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"status":"Time Limit Exceeded","exitStatus":0,"files":{"stdout":"","stderr":"timeout"}}]`))
	}))
	defer srv.Close()

	exec := NewGoJudgeExecutor(workspace, GoJudgeConfig{
		Endpoint:          srv.URL,
		RequestTimeoutSec: 10,
		WorkdirMount:      "/w",
		CPULimitMS:        4000,
		WallLimitMS:       10000,
		MemoryLimitKB:     262144,
		ProcLimit:         128,
		MaxStdoutBytes:    1024,
		MaxStderrBytes:    1024,
	})

	res, err := exec.Execute(context.Background(), &ExecRequest{
		WorkingDir: workspace,
		Timeout:    2 * time.Second,
		Command: Command{
			Display:  "sleep 3",
			UseShell: true,
		},
	})
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if !res.TimedOut {
		t.Fatalf("expected timeout=true")
	}
	if res.ExitCode != -1 {
		t.Fatalf("expected timeout exit code -1, got %d", res.ExitCode)
	}
	if !strings.Contains(string(res.Stderr), "timeout") {
		t.Fatalf("unexpected stderr: %q", string(res.Stderr))
	}
}

func TestGoJudgeExecutorRejectsOutsideWorkspaceWorkingDir(t *testing.T) {
	workspace := t.TempDir()
	exec := NewGoJudgeExecutor(workspace, GoJudgeConfig{
		Endpoint:          "http://127.0.0.1:5050",
		RequestTimeoutSec: 10,
		WorkdirMount:      "/w",
		CPULimitMS:        4000,
		WallLimitMS:       10000,
		MemoryLimitKB:     262144,
		ProcLimit:         128,
		MaxStdoutBytes:    1024,
		MaxStderrBytes:    1024,
	})

	_, err := exec.Execute(context.Background(), &ExecRequest{
		WorkingDir: "/tmp",
		Timeout:    time.Second,
		Command: Command{
			Display:  "echo test",
			UseShell: true,
		},
	})
	if err == nil {
		t.Fatalf("expected error for outside workspace working_dir")
	}
	if !strings.Contains(err.Error(), "within workspace") {
		t.Fatalf("unexpected error: %v", err)
	}
}
