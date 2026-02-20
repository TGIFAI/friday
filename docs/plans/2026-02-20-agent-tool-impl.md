# Agent Tool Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement a new `agent` tool in friday that delegates coding tasks to Claude Code and Codex CLIs via their non-interactive pipe modes.

**Architecture:** New `agentx` package under `internal/agent/tool/` following existing tool conventions. A `Backend` interface abstracts CLI differences. A `SessionManager` tracks active sessions. The `AgentTool` dispatches actions (create/send/status/list/destroy) to backends and session manager.

**Tech Stack:** Go 1.24, `github.com/bytedance/gg/gconv` for type conversion, `github.com/bytedance/sonic` for JSON parsing, `github.com/cloudwego/eino/schema` for ToolInfo.

**Design doc:** `docs/plans/2026-02-20-agent-tool-design.md`

---

### Task 1: Backend Interface and Types

**Files:**
- Create: `internal/agent/tool/agentx/backend.go`
- Test: `internal/agent/tool/agentx/backend_test.go`

**Step 1: Write the test file with basic type assertions**

```go
package agentx

import (
	"context"
	"testing"
)

func TestRunRequestFields(t *testing.T) {
	req := &RunRequest{
		Prompt:       "fix bug",
		WorkingDir:   "/tmp/project",
		SystemPrompt: "be careful",
		MaxTurns:     10,
		ResumeID:     "session-abc",
	}
	if req.Prompt != "fix bug" {
		t.Fatalf("expected prompt 'fix bug', got %q", req.Prompt)
	}
	if req.MaxTurns != 10 {
		t.Fatalf("expected max_turns 10, got %d", req.MaxTurns)
	}
}

func TestRunResultFields(t *testing.T) {
	res := &RunResult{
		CLISessionID: "abc-123",
		Output:       "done",
		ExitCode:     0,
	}
	if res.CLISessionID != "abc-123" {
		t.Fatalf("expected cli_session_id 'abc-123', got %q", res.CLISessionID)
	}
}

// Verify Backend interface is satisfied at compile time by a mock.
type mockBackend struct{}

func (m *mockBackend) Name() string { return "mock" }
func (m *mockBackend) Available() bool { return true }
func (m *mockBackend) Run(_ context.Context, _ *RunRequest) (*RunResult, error) {
	return &RunResult{}, nil
}
func (m *mockBackend) Start(_ context.Context, _ *RunRequest) (*Process, error) {
	return &Process{}, nil
}

var _ Backend = (*mockBackend)(nil)
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/dave/go/src/github.com/tgifai/friday && go test ./internal/agent/tool/agentx/... -v -run TestRunRequest`
Expected: FAIL — package does not exist yet

**Step 3: Write backend.go with types and interface**

```go
package agentx

import (
	"context"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
)

// Backend abstracts CLI differences between Claude Code and Codex.
type Backend interface {
	Name() string
	Available() bool
	Run(ctx context.Context, req *RunRequest) (*RunResult, error)
	Start(ctx context.Context, req *RunRequest) (*Process, error)
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
	stdout *limitedBuffer
	stderr *limitedBuffer
	done   chan struct{}

	mu       sync.RWMutex
	exitCode int
	waitErr  string
	finished bool
}

// Done returns a channel that closes when the process exits.
func (p *Process) Done() <-chan struct{} { return p.done }

// Result collects the process output after it finishes.
func (p *Process) Result() *RunResult {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return &RunResult{
		Output:   p.stdout.String(),
		ExitCode: p.exitCode,
	}
}

// Kill terminates the process.
func (p *Process) Kill() {
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Signal(os.Kill)
	}
}

// limitedBuffer keeps only the first N bytes, then discards.
// Same pattern as shellx/process.go limitedBuffer.
type limitedBuffer struct {
	max       int
	data      []byte
	truncated bool
}

func newLimitedBuffer(max int) *limitedBuffer {
	return &limitedBuffer{max: max, data: make([]byte, 0, min(max, 64*1024))}
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.truncated {
		return len(p), nil
	}
	remaining := b.max - len(b.data)
	if remaining <= 0 {
		b.truncated = true
		return len(p), nil
	}
	if len(p) > remaining {
		b.data = append(b.data, p[:remaining]...)
		b.truncated = true
	} else {
		b.data = append(b.data, p...)
	}
	return len(p), nil
}

func (b *limitedBuffer) String() string { return string(b.data) }
func (b *limitedBuffer) Bytes() []byte  { return b.data }

// seq is a package-level counter for generating unique session IDs.
var seq atomic.Int64
```

**Step 4: Run tests to verify they pass**

Run: `cd /Users/dave/go/src/github.com/tgifai/friday && go test ./internal/agent/tool/agentx/... -v`
Expected: PASS

**Step 5: Commit**

```bash
cd /Users/dave/go/src/github.com/tgifai/friday
git add internal/agent/tool/agentx/backend.go internal/agent/tool/agentx/backend_test.go
git commit -m "feat(agentx): add Backend interface and shared types"
```

---

### Task 2: Claude Code Backend

**Files:**
- Create: `internal/agent/tool/agentx/claude_code.go`
- Create: `internal/agent/tool/agentx/claude_code_test.go`

**Step 1: Write the test**

```go
package agentx

import (
	"context"
	"os/exec"
	"testing"
)

func TestClaudeCodeBackendName(t *testing.T) {
	b := &ClaudeCodeBackend{}
	if b.Name() != "claude-code" {
		t.Fatalf("expected name 'claude-code', got %q", b.Name())
	}
}

func TestClaudeCodeBackendAvailable(t *testing.T) {
	b := &ClaudeCodeBackend{}
	// Available should match whether "claude" is in PATH
	_, lookErr := exec.LookPath("claude")
	if b.Available() != (lookErr == nil) {
		t.Fatalf("Available() mismatch with LookPath result")
	}
}

func TestClaudeCodeBuildArgs(t *testing.T) {
	b := &ClaudeCodeBackend{}
	tests := []struct {
		name string
		req  *RunRequest
		want []string
	}{
		{
			name: "basic",
			req:  &RunRequest{Prompt: "hello"},
			want: []string{"-p", "hello", "--dangerously-skip-permissions", "--output-format", "json"},
		},
		{
			name: "with resume",
			req:  &RunRequest{Prompt: "hello", ResumeID: "sess-1"},
			want: []string{"-p", "hello", "--dangerously-skip-permissions", "--output-format", "json", "--resume", "sess-1"},
		},
		{
			name: "with system prompt",
			req:  &RunRequest{Prompt: "hello", SystemPrompt: "be safe"},
			want: []string{"-p", "hello", "--dangerously-skip-permissions", "--output-format", "json", "--append-system-prompt", "be safe"},
		},
		{
			name: "with max turns",
			req:  &RunRequest{Prompt: "hello", MaxTurns: 10},
			want: []string{"-p", "hello", "--dangerously-skip-permissions", "--output-format", "json", "--max-turns", "10"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := b.buildArgs(tt.req)
			if len(got) != len(tt.want) {
				t.Fatalf("args length: got %d, want %d\ngot:  %v\nwant: %v", len(got), len(tt.want), got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("args[%d]: got %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestClaudeCodeParseResult(t *testing.T) {
	b := &ClaudeCodeBackend{}

	t.Run("valid json", func(t *testing.T) {
		raw := `{"result":"all fixed","session_id":"abc-123"}`
		res, err := b.parseResult(raw, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.CLISessionID != "abc-123" {
			t.Fatalf("expected session_id 'abc-123', got %q", res.CLISessionID)
		}
		if res.Output != "all fixed" {
			t.Fatalf("expected output 'all fixed', got %q", res.Output)
		}
	})

	t.Run("invalid json fallback", func(t *testing.T) {
		raw := "not json at all"
		res, err := b.parseResult(raw, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Output != "not json at all" {
			t.Fatalf("expected raw fallback, got %q", res.Output)
		}
	})

	t.Run("non-zero exit code", func(t *testing.T) {
		raw := `{"result":"partial","session_id":"x"}`
		res, err := b.parseResult(raw, 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.ExitCode != 1 {
			t.Fatalf("expected exit code 1, got %d", res.ExitCode)
		}
	})
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/dave/go/src/github.com/tgifai/friday && go test ./internal/agent/tool/agentx/... -v -run TestClaudeCode`
Expected: FAIL — ClaudeCodeBackend not defined

**Step 3: Write claude_code.go**

```go
package agentx

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"

	"github.com/bytedance/sonic"
)

const maxOutputBytes = 1 << 20 // 1 MiB

// ClaudeCodeBackend wraps the `claude` CLI in non-interactive mode.
type ClaudeCodeBackend struct{}

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

func (b *ClaudeCodeBackend) parseResult(raw string, exitCode int) (*RunResult, error) {
	var parsed struct {
		Result    string `json:"result"`
		SessionID string `json:"session_id"`
	}
	if err := sonic.UnmarshalString(raw, &parsed); err != nil || parsed.Result == "" {
		// Fallback: treat raw output as the result
		return &RunResult{Output: raw, ExitCode: exitCode}, nil
	}
	return &RunResult{
		CLISessionID: parsed.SessionID,
		Output:       parsed.Result,
		ExitCode:     exitCode,
	}, nil
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

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if !isExitError(err, &exitErr) {
			return nil, fmt.Errorf("claude CLI failed: %w (stderr: %s)", err, stderr.String())
		}
		// Non-zero exit — still parse output
	}

	return b.parseResult(stdout.String(), cmd.ProcessState.ExitCode())
}

func (b *ClaudeCodeBackend) Start(ctx context.Context, req *RunRequest) (*Process, error) {
	args := b.buildArgs(req)
	cmd := exec.CommandContext(ctx, "claude", args...)
	if req.WorkingDir != "" {
		cmd.Dir = req.WorkingDir
	}

	stdoutBuf := newLimitedBuffer(maxOutputBytes)
	stderrBuf := newLimitedBuffer(maxOutputBytes)
	cmd.Stdout = stdoutBuf
	cmd.Stderr = stderrBuf

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("claude CLI start failed: %w", err)
	}

	p := &Process{
		cmd:    cmd,
		stdout: stdoutBuf,
		stderr: stderrBuf,
		done:   make(chan struct{}),
	}

	go func() {
		defer close(p.done)
		waitErr := cmd.Wait()
		p.mu.Lock()
		defer p.mu.Unlock()
		p.finished = true
		if cmd.ProcessState != nil {
			p.exitCode = cmd.ProcessState.ExitCode()
		}
		if waitErr != nil {
			p.waitErr = waitErr.Error()
		}
	}()

	return p, nil
}

// isExitError checks if err is an *exec.ExitError.
func isExitError(err error, target **exec.ExitError) bool {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		*target = exitErr
		return true
	}
	return false
}
```

Note: add `"errors"` to imports (needed for `errors.As`). Remove unused `"bytes"` import.

**Step 4: Run tests to verify they pass**

Run: `cd /Users/dave/go/src/github.com/tgifai/friday && go test ./internal/agent/tool/agentx/... -v -run TestClaudeCode`
Expected: PASS

**Step 5: Commit**

```bash
cd /Users/dave/go/src/github.com/tgifai/friday
git add internal/agent/tool/agentx/claude_code.go internal/agent/tool/agentx/claude_code_test.go
git commit -m "feat(agentx): add Claude Code CLI backend"
```

---

### Task 3: Codex Backend

**Files:**
- Create: `internal/agent/tool/agentx/codex.go`
- Create: `internal/agent/tool/agentx/codex_test.go`

**Step 1: Write the test**

```go
package agentx

import (
	"os/exec"
	"testing"
)

func TestCodexBackendName(t *testing.T) {
	b := &CodexBackend{}
	if b.Name() != "codex" {
		t.Fatalf("expected name 'codex', got %q", b.Name())
	}
}

func TestCodexBackendAvailable(t *testing.T) {
	b := &CodexBackend{}
	_, lookErr := exec.LookPath("codex")
	if b.Available() != (lookErr == nil) {
		t.Fatalf("Available() mismatch with LookPath result")
	}
}

func TestCodexBuildArgs(t *testing.T) {
	b := &CodexBackend{}
	tests := []struct {
		name string
		req  *RunRequest
		want []string
	}{
		{
			name: "basic",
			req:  &RunRequest{Prompt: "hello"},
			want: []string{"exec", "hello", "--json", "--dangerously-bypass-approvals-and-sandbox"},
		},
		{
			name: "with resume",
			req:  &RunRequest{Prompt: "hello", ResumeID: "sess-1"},
			want: []string{"exec", "resume", "sess-1", "hello", "--json", "--dangerously-bypass-approvals-and-sandbox"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := b.buildArgs(tt.req)
			if len(got) != len(tt.want) {
				t.Fatalf("args length: got %d, want %d\ngot:  %v\nwant: %v", len(got), len(tt.want), got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("args[%d]: got %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestCodexParseJSONL(t *testing.T) {
	b := &CodexBackend{}

	t.Run("extracts last assistant message", func(t *testing.T) {
		jsonl := `{"type":"thread.started","thread_id":"t-1"}
{"type":"item.created","item":{"type":"message","role":"assistant","content":[{"type":"text","text":"working on it..."}]}}
{"type":"item.created","item":{"type":"message","role":"assistant","content":[{"type":"text","text":"all done!"}]}}
{"type":"turn.completed"}`
		res := b.parseJSONL(jsonl, 0)
		if res.Output != "all done!" {
			t.Fatalf("expected 'all done!', got %q", res.Output)
		}
		if res.CLISessionID != "t-1" {
			t.Fatalf("expected thread_id 't-1', got %q", res.CLISessionID)
		}
	})

	t.Run("fallback on no messages", func(t *testing.T) {
		jsonl := `{"type":"thread.started","thread_id":"t-2"}`
		res := b.parseJSONL(jsonl, 0)
		if res.Output != jsonl {
			t.Fatalf("expected raw fallback")
		}
	})
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/dave/go/src/github.com/tgifai/friday && go test ./internal/agent/tool/agentx/... -v -run TestCodex`
Expected: FAIL — CodexBackend not defined

**Step 3: Write codex.go**

```go
package agentx

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/bytedance/sonic"
)

// CodexBackend wraps the `codex` CLI in non-interactive mode.
type CodexBackend struct{}

func (b *CodexBackend) Name() string { return "codex" }

func (b *CodexBackend) Available() bool {
	_, err := exec.LookPath("codex")
	return err == nil
}

func (b *CodexBackend) buildArgs(req *RunRequest) []string {
	if req.ResumeID != "" {
		return []string{"exec", "resume", req.ResumeID, req.Prompt, "--json", "--dangerously-bypass-approvals-and-sandbox"}
	}
	return []string{"exec", req.Prompt, "--json", "--dangerously-bypass-approvals-and-sandbox"}
}

func (b *CodexBackend) parseJSONL(raw string, exitCode int) *RunResult {
	var sessionID string
	var lastMessage string

	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var event struct {
			Type     string `json:"type"`
			ThreadID string `json:"thread_id"`
			Item     struct {
				Type    string `json:"type"`
				Role    string `json:"role"`
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"item"`
		}
		if err := sonic.UnmarshalString(line, &event); err != nil {
			continue
		}
		if event.ThreadID != "" {
			sessionID = event.ThreadID
		}
		if event.Item.Role == "assistant" && len(event.Item.Content) > 0 {
			for _, c := range event.Item.Content {
				if c.Type == "text" && c.Text != "" {
					lastMessage = c.Text
				}
			}
		}
	}

	if lastMessage == "" {
		return &RunResult{CLISessionID: sessionID, Output: raw, ExitCode: exitCode}
	}
	return &RunResult{CLISessionID: sessionID, Output: lastMessage, ExitCode: exitCode}
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

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if !isExitError(err, &exitErr) {
			return nil, fmt.Errorf("codex CLI failed: %w (stderr: %s)", err, stderr.String())
		}
	}

	return b.parseJSONL(stdout.String(), cmd.ProcessState.ExitCode()), nil
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
		return nil, fmt.Errorf("codex CLI start failed: %w", err)
	}

	p := &Process{
		cmd:    cmd,
		stdout: stdoutBuf,
		stderr: stderrBuf,
		done:   make(chan struct{}),
	}

	go func() {
		defer close(p.done)
		waitErr := cmd.Wait()
		p.mu.Lock()
		defer p.mu.Unlock()
		p.finished = true
		if cmd.ProcessState != nil {
			p.exitCode = cmd.ProcessState.ExitCode()
		}
		if waitErr != nil {
			p.waitErr = waitErr.Error()
		}
	}()

	return p, nil
}
```

**Step 4: Run tests to verify they pass**

Run: `cd /Users/dave/go/src/github.com/tgifai/friday && go test ./internal/agent/tool/agentx/... -v -run TestCodex`
Expected: PASS

**Step 5: Commit**

```bash
cd /Users/dave/go/src/github.com/tgifai/friday
git add internal/agent/tool/agentx/codex.go internal/agent/tool/agentx/codex_test.go
git commit -m "feat(agentx): add Codex CLI backend"
```

---

### Task 4: Session Manager

**Files:**
- Create: `internal/agent/tool/agentx/session.go`
- Create: `internal/agent/tool/agentx/session_test.go`

**Step 1: Write the test**

```go
package agentx

import (
	"testing"
)

func TestSessionManagerCreate(t *testing.T) {
	sm := NewSessionManager(4)
	s := sm.Create("claude-code", "/tmp/work")
	if s.ID == "" {
		t.Fatal("expected non-empty session ID")
	}
	if s.Backend != "claude-code" {
		t.Fatalf("expected backend 'claude-code', got %q", s.Backend)
	}
	if s.Status != StatusRunning {
		t.Fatalf("expected status 'running', got %q", s.Status)
	}
}

func TestSessionManagerGet(t *testing.T) {
	sm := NewSessionManager(4)
	s := sm.Create("claude-code", "/tmp")

	got, ok := sm.Get(s.ID)
	if !ok {
		t.Fatal("expected to find session")
	}
	if got.ID != s.ID {
		t.Fatalf("ID mismatch: %q vs %q", got.ID, s.ID)
	}

	_, ok = sm.Get("nonexistent")
	if ok {
		t.Fatal("expected not to find session")
	}
}

func TestSessionManagerList(t *testing.T) {
	sm := NewSessionManager(4)
	sm.Create("claude-code", "/tmp")
	sm.Create("codex", "/tmp")

	list := sm.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(list))
	}
}

func TestSessionManagerDestroy(t *testing.T) {
	sm := NewSessionManager(4)
	s := sm.Create("claude-code", "/tmp")
	sm.Destroy(s.ID)

	_, ok := sm.Get(s.ID)
	if ok {
		t.Fatal("expected session to be destroyed")
	}
}

func TestSessionManagerMaxSessions(t *testing.T) {
	sm := NewSessionManager(2)
	sm.Create("claude-code", "/tmp")
	sm.Create("codex", "/tmp")

	_, err := sm.CreateWithLimit("claude-code", "/tmp")
	if err == nil {
		t.Fatal("expected error when max sessions reached")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/dave/go/src/github.com/tgifai/friday && go test ./internal/agent/tool/agentx/... -v -run TestSessionManager`
Expected: FAIL — SessionManager not defined

**Step 3: Write session.go**

```go
package agentx

import (
	"fmt"
	"sync"
	"time"
)

const (
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
)

// Session represents an active CLI agent session.
type Session struct {
	ID           string
	Backend      string
	CLISessionID string
	Status       string
	WorkingDir   string
	CreatedAt    time.Time
	LastOutput   string
	process      *Process // nil for sync sessions
}

// SessionManager tracks active agent sessions.
type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	max      int
}

func NewSessionManager(maxSessions int) *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*Session),
		max:      maxSessions,
	}
}

// Create adds a new session without checking limits. Used internally.
func (sm *SessionManager) Create(backend, workingDir string) *Session {
	id := fmt.Sprintf("as-%d", seq.Add(1))
	s := &Session{
		ID:        id,
		Backend:   backend,
		Status:    StatusRunning,
		WorkingDir: workingDir,
		CreatedAt: time.Now(),
	}
	sm.mu.Lock()
	sm.sessions[id] = s
	sm.mu.Unlock()
	return s
}

// CreateWithLimit creates a session but returns an error if max is reached.
func (sm *SessionManager) CreateWithLimit(backend, workingDir string) (*Session, error) {
	sm.mu.RLock()
	count := len(sm.sessions)
	sm.mu.RUnlock()
	if count >= sm.max {
		return nil, fmt.Errorf("max sessions (%d) reached, destroy one first", sm.max)
	}
	return sm.Create(backend, workingDir), nil
}

func (sm *SessionManager) Get(id string) (*Session, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	s, ok := sm.sessions[id]
	return s, ok
}

func (sm *SessionManager) List() []*Session {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	list := make([]*Session, 0, len(sm.sessions))
	for _, s := range sm.sessions {
		list = append(list, s)
	}
	return list
}

func (sm *SessionManager) Destroy(id string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if s, ok := sm.sessions[id]; ok {
		if s.process != nil {
			s.process.Kill()
		}
		delete(sm.sessions, id)
	}
}
```

**Step 4: Run tests to verify they pass**

Run: `cd /Users/dave/go/src/github.com/tgifai/friday && go test ./internal/agent/tool/agentx/... -v -run TestSessionManager`
Expected: PASS

**Step 5: Commit**

```bash
cd /Users/dave/go/src/github.com/tgifai/friday
git add internal/agent/tool/agentx/session.go internal/agent/tool/agentx/session_test.go
git commit -m "feat(agentx): add session manager"
```

---

### Task 5: Agent Tool — ToolInfo and Action Dispatch

**Files:**
- Create: `internal/agent/tool/agentx/agent.go`
- Create: `internal/agent/tool/agentx/agent_test.go`

**Step 1: Write the test**

```go
package agentx

import (
	"context"
	"strings"
	"testing"
)

func TestAgentToolName(t *testing.T) {
	tool := NewAgentTool("")
	if tool.Name() != "agent" {
		t.Fatalf("expected name 'agent', got %q", tool.Name())
	}
}

func TestAgentToolInfo(t *testing.T) {
	tool := NewAgentTool("")
	info := tool.ToolInfo()
	if info.Name != "agent" {
		t.Fatalf("expected ToolInfo name 'agent', got %q", info.Name)
	}
	if info.ParamsOneOf == nil {
		t.Fatal("expected ParamsOneOf to be set")
	}
}

func TestAgentToolExecuteMissingAction(t *testing.T) {
	tool := NewAgentTool("")
	_, err := tool.Execute(context.Background(), map[string]interface{}{})
	if err == nil || !strings.Contains(err.Error(), "action is required") {
		t.Fatalf("expected 'action is required' error, got %v", err)
	}
}

func TestAgentToolExecuteUnknownAction(t *testing.T) {
	tool := NewAgentTool("")
	_, err := tool.Execute(context.Background(), map[string]interface{}{"action": "fly"})
	if err == nil || !strings.Contains(err.Error(), "unsupported action") {
		t.Fatalf("expected 'unsupported action' error, got %v", err)
	}
}

func TestAgentToolExecuteCreateMissingBackend(t *testing.T) {
	tool := NewAgentTool("")
	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"action": "create",
		"prompt": "hello",
	})
	if err == nil || !strings.Contains(err.Error(), "backend is required") {
		t.Fatalf("expected 'backend is required' error, got %v", err)
	}
}

func TestAgentToolExecuteCreateUnavailableBackend(t *testing.T) {
	tool := NewAgentTool("")
	// Use a backend name that has no CLI installed (unlikely to have "nonexistent" in PATH)
	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"action":  "create",
		"backend": "nonexistent",
		"prompt":  "hello",
	})
	if err == nil || !strings.Contains(err.Error(), "unknown backend") {
		t.Fatalf("expected 'unknown backend' error, got %v", err)
	}
}

func TestAgentToolExecuteList(t *testing.T) {
	tool := NewAgentTool("")
	res, err := tool.Execute(context.Background(), map[string]interface{}{"action": "list"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := res.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map result, got %T", res)
	}
	sessions, ok := m["sessions"]
	if !ok {
		t.Fatal("expected 'sessions' key in result")
	}
	list, ok := sessions.([]map[string]interface{})
	if !ok {
		t.Fatalf("expected []map[string]interface{}, got %T", sessions)
	}
	if len(list) != 0 {
		t.Fatalf("expected empty session list, got %d", len(list))
	}
}

func TestAgentToolExecuteStatusNotFound(t *testing.T) {
	tool := NewAgentTool("")
	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"action":     "status",
		"session_id": "as-999",
	})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected 'not found' error, got %v", err)
	}
}

func TestAgentToolExecuteDestroyNotFound(t *testing.T) {
	tool := NewAgentTool("")
	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"action":     "destroy",
		"session_id": "as-999",
	})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected 'not found' error, got %v", err)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/dave/go/src/github.com/tgifai/friday && go test ./internal/agent/tool/agentx/... -v -run TestAgentTool`
Expected: FAIL — AgentTool not defined

**Step 3: Write agent.go**

```go
package agentx

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bytedance/gg/gconv"
	"github.com/cloudwego/eino/schema"

	"github.com/tgifai/friday/internal/pkg/logs"
)

const (
	maxSessions    = 8
	defaultTimeout = 600 * time.Second
)

// AgentTool delegates tasks to external CLI agents (Claude Code, Codex).
type AgentTool struct {
	workspace string
	backends  map[string]Backend
	sessions  *SessionManager
}

func NewAgentTool(workspace string) *AgentTool {
	backends := map[string]Backend{
		"claude-code": &ClaudeCodeBackend{},
		"codex":       &CodexBackend{},
	}
	return &AgentTool{
		workspace: workspace,
		backends:  backends,
		sessions:  NewSessionManager(maxSessions),
	}
}

func (t *AgentTool) Name() string        { return "agent" }
func (t *AgentTool) Description() string {
	return "Delegate coding tasks to CLI agents (Claude Code or Codex). Supports creating sessions, sending follow-up messages, checking status, and managing session lifecycle."
}

func (t *AgentTool) ToolInfo() *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: t.Name(),
		Desc: t.Description(),
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"action": {
				Type:     schema.String,
				Desc:     `Action: "create" (new session), "send" (follow-up message), "status" (check async), "list" (all sessions), "destroy" (terminate)`,
				Required: true,
				Enum:     []string{"create", "send", "status", "list", "destroy"},
			},
			"backend": {
				Type: schema.String,
				Desc: `CLI backend: "claude-code" or "codex". Required for "create".`,
				Enum: []string{"claude-code", "codex"},
			},
			"prompt": {
				Type: schema.String,
				Desc: `The task/prompt to send to the CLI agent. Required for "create" and "send".`,
			},
			"session_id": {
				Type: schema.String,
				Desc: `Session ID (e.g. "as-1"). Required for "send", "status", "destroy".`,
			},
			"working_dir": {
				Type: schema.String,
				Desc: `Working directory for the CLI agent. Optional, defaults to agent workspace.`,
			},
			"system_prompt": {
				Type: schema.String,
				Desc: `Additional system prompt to append. Optional, only for "create".`,
			},
			"max_turns": {
				Type: schema.Integer,
				Desc: `Maximum agentic turns. Optional, only for "create".`,
			},
			"async": {
				Type: schema.Boolean,
				Desc: `If true, run in background and return immediately. Use "status" to poll. Default false.`,
			},
		}),
	}
}

func (t *AgentTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	action := strings.ToLower(strings.TrimSpace(gconv.To[string](args["action"])))
	if action == "" {
		return nil, fmt.Errorf("action is required")
	}

	switch action {
	case "create":
		return t.executeCreate(ctx, args)
	case "send":
		return t.executeSend(ctx, args)
	case "status":
		return t.executeStatus(args)
	case "list":
		return t.executeList()
	case "destroy":
		return t.executeDestroy(args)
	default:
		return nil, fmt.Errorf("unsupported action: %s", action)
	}
}

func (t *AgentTool) executeCreate(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	backendName := strings.TrimSpace(gconv.To[string](args["backend"]))
	if backendName == "" {
		return nil, fmt.Errorf("backend is required for create action")
	}
	prompt := gconv.To[string](args["prompt"])
	if prompt == "" {
		return nil, fmt.Errorf("prompt is required for create action")
	}

	backend, ok := t.backends[backendName]
	if !ok {
		return nil, fmt.Errorf("unknown backend: %s (available: claude-code, codex)", backendName)
	}
	if !backend.Available() {
		return nil, fmt.Errorf("%s CLI not found in PATH", backendName)
	}

	workingDir := strings.TrimSpace(gconv.To[string](args["working_dir"]))
	if workingDir == "" {
		workingDir = t.workspace
	}

	req := &RunRequest{
		Prompt:       prompt,
		WorkingDir:   workingDir,
		SystemPrompt: gconv.To[string](args["system_prompt"]),
		MaxTurns:     gconv.To[int](args["max_turns"]),
	}

	sess, err := t.sessions.CreateWithLimit(backendName, workingDir)
	if err != nil {
		return nil, err
	}

	async := gconv.To[bool](args["async"])
	logs.CtxInfo(ctx, "[tool:agent] create session %s, backend=%s, async=%v", sess.ID, backendName, async)

	if async {
		proc, err := backend.Start(ctx, req)
		if err != nil {
			t.sessions.Destroy(sess.ID)
			return nil, err
		}
		sess.process = proc
		go func() {
			<-proc.Done()
			res := proc.Result()
			sess.CLISessionID = res.CLISessionID
			sess.LastOutput = res.Output
			if res.ExitCode == 0 {
				sess.Status = StatusCompleted
			} else {
				sess.Status = StatusFailed
			}
		}()
		return map[string]interface{}{
			"session_id": sess.ID,
			"backend":    backendName,
			"status":     StatusRunning,
		}, nil
	}

	// Sync execution
	timeoutCtx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	res, err := backend.Run(timeoutCtx, req)
	if err != nil {
		sess.Status = StatusFailed
		return nil, fmt.Errorf("agent execution failed: %w", err)
	}

	sess.CLISessionID = res.CLISessionID
	sess.LastOutput = res.Output
	sess.Status = StatusCompleted

	return map[string]interface{}{
		"session_id":     sess.ID,
		"backend":        backendName,
		"cli_session_id": res.CLISessionID,
		"status":         StatusCompleted,
		"result":         res.Output,
	}, nil
}

func (t *AgentTool) executeSend(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	sessionID := strings.TrimSpace(gconv.To[string](args["session_id"]))
	if sessionID == "" {
		return nil, fmt.Errorf("session_id is required for send action")
	}
	prompt := gconv.To[string](args["prompt"])
	if prompt == "" {
		return nil, fmt.Errorf("prompt is required for send action")
	}

	sess, ok := t.sessions.Get(sessionID)
	if !ok {
		return nil, fmt.Errorf("session %s not found", sessionID)
	}
	if sess.CLISessionID == "" {
		return nil, fmt.Errorf("session %s has no CLI session ID (was it completed?)", sessionID)
	}

	backend, ok := t.backends[sess.Backend]
	if !ok {
		return nil, fmt.Errorf("backend %s not available", sess.Backend)
	}

	req := &RunRequest{
		Prompt:     prompt,
		WorkingDir: sess.WorkingDir,
		ResumeID:   sess.CLISessionID,
	}

	async := gconv.To[bool](args["async"])
	logs.CtxInfo(ctx, "[tool:agent] send to session %s, resume=%s, async=%v", sessionID, sess.CLISessionID, async)

	if async {
		sess.Status = StatusRunning
		proc, err := backend.Start(ctx, req)
		if err != nil {
			return nil, err
		}
		sess.process = proc
		go func() {
			<-proc.Done()
			res := proc.Result()
			if res.CLISessionID != "" {
				sess.CLISessionID = res.CLISessionID
			}
			sess.LastOutput = res.Output
			if res.ExitCode == 0 {
				sess.Status = StatusCompleted
			} else {
				sess.Status = StatusFailed
			}
		}()
		return map[string]interface{}{
			"session_id": sess.ID,
			"status":     StatusRunning,
		}, nil
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	sess.Status = StatusRunning
	res, err := backend.Run(timeoutCtx, req)
	if err != nil {
		sess.Status = StatusFailed
		return nil, fmt.Errorf("agent send failed: %w", err)
	}

	if res.CLISessionID != "" {
		sess.CLISessionID = res.CLISessionID
	}
	sess.LastOutput = res.Output
	sess.Status = StatusCompleted

	return map[string]interface{}{
		"session_id":     sess.ID,
		"cli_session_id": sess.CLISessionID,
		"status":         StatusCompleted,
		"result":         res.Output,
	}, nil
}

func (t *AgentTool) executeStatus(args map[string]interface{}) (interface{}, error) {
	sessionID := strings.TrimSpace(gconv.To[string](args["session_id"]))
	if sessionID == "" {
		return nil, fmt.Errorf("session_id is required for status action")
	}

	sess, ok := t.sessions.Get(sessionID)
	if !ok {
		return nil, fmt.Errorf("session %s not found", sessionID)
	}

	result := map[string]interface{}{
		"session_id": sess.ID,
		"backend":    sess.Backend,
		"status":     sess.Status,
		"created_at": sess.CreatedAt.Format(time.RFC3339),
	}
	if sess.CLISessionID != "" {
		result["cli_session_id"] = sess.CLISessionID
	}
	if sess.LastOutput != "" {
		result["result"] = sess.LastOutput
	}
	return result, nil
}

func (t *AgentTool) executeList() (interface{}, error) {
	sessions := t.sessions.List()
	list := make([]map[string]interface{}, 0, len(sessions))
	for _, s := range sessions {
		list = append(list, map[string]interface{}{
			"session_id": s.ID,
			"backend":    s.Backend,
			"status":     s.Status,
			"created_at": s.CreatedAt.Format(time.RFC3339),
		})
	}
	return map[string]interface{}{"sessions": list}, nil
}

func (t *AgentTool) executeDestroy(args map[string]interface{}) (interface{}, error) {
	sessionID := strings.TrimSpace(gconv.To[string](args["session_id"]))
	if sessionID == "" {
		return nil, fmt.Errorf("session_id is required for destroy action")
	}

	_, ok := t.sessions.Get(sessionID)
	if !ok {
		return nil, fmt.Errorf("session %s not found", sessionID)
	}

	t.sessions.Destroy(sessionID)
	return map[string]interface{}{"success": true, "session_id": sessionID}, nil
}
```

**Step 4: Run tests to verify they pass**

Run: `cd /Users/dave/go/src/github.com/tgifai/friday && go test ./internal/agent/tool/agentx/... -v -run TestAgentTool`
Expected: PASS

**Step 5: Commit**

```bash
cd /Users/dave/go/src/github.com/tgifai/friday
git add internal/agent/tool/agentx/agent.go internal/agent/tool/agentx/agent_test.go
git commit -m "feat(agentx): add AgentTool with action dispatch and session lifecycle"
```

---

### Task 6: Register Tool in Agent

**Files:**
- Modify: `internal/agent/agent.go:13` (add import)
- Modify: `internal/agent/agent.go:97-99` (add registration)

**Step 1: Add import and registration**

In `internal/agent/agent.go`, add to imports:
```go
"github.com/tgifai/friday/internal/agent/tool/agentx"
```

Add before `return nil` at line 100:
```go
	// agent delegation tools
	_ = ag.tools.Register(agentx.NewAgentTool(ag.workspace))
```

**Step 2: Run full test suite**

Run: `cd /Users/dave/go/src/github.com/tgifai/friday && go build ./...`
Expected: Build succeeds

Run: `cd /Users/dave/go/src/github.com/tgifai/friday && go test ./internal/agent/... -v -count=1`
Expected: PASS

**Step 3: Commit**

```bash
cd /Users/dave/go/src/github.com/tgifai/friday
git add internal/agent/agent.go
git commit -m "feat: register agentx tool in agent init"
```

---

### Task 7: Full Package Tests and Cleanup

**Files:**
- Modify: `internal/agent/tool/agentx/agent_test.go` (add integration-style tests)

**Step 1: Add integration test with mock backend**

Append to `agent_test.go`:

```go
func TestAgentToolCreateWithMockBackend(t *testing.T) {
	tool := NewAgentTool("")
	// Inject a mock backend for testing
	tool.backends["mock"] = &testBackend{
		name:      "mock",
		available: true,
		runResult: &RunResult{CLISessionID: "mock-sess-1", Output: "task done", ExitCode: 0},
	}

	res, err := tool.Execute(context.Background(), map[string]interface{}{
		"action":  "create",
		"backend": "mock",
		"prompt":  "do something",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := res.(map[string]interface{})
	if m["status"] != StatusCompleted {
		t.Fatalf("expected completed, got %v", m["status"])
	}
	if m["result"] != "task done" {
		t.Fatalf("expected 'task done', got %v", m["result"])
	}
	if m["cli_session_id"] != "mock-sess-1" {
		t.Fatalf("expected cli_session_id 'mock-sess-1', got %v", m["cli_session_id"])
	}

	// Verify session was created
	listRes, _ := tool.Execute(context.Background(), map[string]interface{}{"action": "list"})
	listMap := listRes.(map[string]interface{})
	sessions := listMap["sessions"].([]map[string]interface{})
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
}

func TestAgentToolSendWithMockBackend(t *testing.T) {
	tool := NewAgentTool("")
	tool.backends["mock"] = &testBackend{
		name:      "mock",
		available: true,
		runResult: &RunResult{CLISessionID: "mock-sess-1", Output: "created", ExitCode: 0},
	}

	// Create session
	res, _ := tool.Execute(context.Background(), map[string]interface{}{
		"action": "create", "backend": "mock", "prompt": "start",
	})
	sessionID := res.(map[string]interface{})["session_id"].(string)

	// Update mock for send
	tool.backends["mock"] = &testBackend{
		name:      "mock",
		available: true,
		runResult: &RunResult{CLISessionID: "mock-sess-1", Output: "follow-up done", ExitCode: 0},
	}

	// Send follow-up
	res, err := tool.Execute(context.Background(), map[string]interface{}{
		"action": "send", "session_id": sessionID, "prompt": "continue",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := res.(map[string]interface{})
	if m["result"] != "follow-up done" {
		t.Fatalf("expected 'follow-up done', got %v", m["result"])
	}
}

func TestAgentToolDestroyWithMockBackend(t *testing.T) {
	tool := NewAgentTool("")
	tool.backends["mock"] = &testBackend{
		name:      "mock",
		available: true,
		runResult: &RunResult{CLISessionID: "m-1", Output: "ok", ExitCode: 0},
	}

	res, _ := tool.Execute(context.Background(), map[string]interface{}{
		"action": "create", "backend": "mock", "prompt": "start",
	})
	sessionID := res.(map[string]interface{})["session_id"].(string)

	// Destroy
	res, err := tool.Execute(context.Background(), map[string]interface{}{
		"action": "destroy", "session_id": sessionID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.(map[string]interface{})["success"] != true {
		t.Fatal("expected success: true")
	}

	// Verify gone
	_, err = tool.Execute(context.Background(), map[string]interface{}{
		"action": "status", "session_id": sessionID,
	})
	if err == nil {
		t.Fatal("expected error for destroyed session")
	}
}

// testBackend is a mock Backend for unit testing.
type testBackend struct {
	name      string
	available bool
	runResult *RunResult
	runErr    error
}

func (b *testBackend) Name() string      { return b.name }
func (b *testBackend) Available() bool    { return b.available }
func (b *testBackend) Run(_ context.Context, _ *RunRequest) (*RunResult, error) {
	return b.runResult, b.runErr
}
func (b *testBackend) Start(_ context.Context, _ *RunRequest) (*Process, error) {
	return &Process{done: make(chan struct{})}, nil
}
```

**Step 2: Run full test suite**

Run: `cd /Users/dave/go/src/github.com/tgifai/friday && go test ./internal/agent/tool/agentx/... -v -count=1`
Expected: ALL PASS

**Step 3: Run go vet and build**

Run: `cd /Users/dave/go/src/github.com/tgifai/friday && go vet ./internal/agent/tool/agentx/... && go build ./...`
Expected: No issues

**Step 4: Commit**

```bash
cd /Users/dave/go/src/github.com/tgifai/friday
git add internal/agent/tool/agentx/
git commit -m "test(agentx): add integration tests with mock backend"
```

---

### Task 8: Final Verification

**Step 1: Run entire project test suite**

Run: `cd /Users/dave/go/src/github.com/tgifai/friday && go test ./... -count=1`
Expected: ALL PASS (or only pre-existing failures unrelated to agentx)

**Step 2: Verify build**

Run: `cd /Users/dave/go/src/github.com/tgifai/friday && go build -trimpath -ldflags="-s -w" ./cmd/friday`
Expected: Binary builds successfully

**Step 3: Review all changes**

Run: `cd /Users/dave/go/src/github.com/tgifai/friday && git log --oneline -5`
Expected: See all commits from Tasks 1-7
