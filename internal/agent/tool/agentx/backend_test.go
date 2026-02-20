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

func (m *mockBackend) Name() string                                          { return "mock" }
func (m *mockBackend) Available() bool                                       { return true }
func (m *mockBackend) Run(_ context.Context, _ *RunRequest) (*RunResult, error) {
	return &RunResult{}, nil
}
func (m *mockBackend) Start(_ context.Context, _ *RunRequest) (*Process, error) {
	return &Process{}, nil
}

var _ Backend = (*mockBackend)(nil)

func TestProcessZeroValue(t *testing.T) {
	// Process must be safe to use when constructed as a zero value
	// (e.g. &Process{} in mocks/tests with nil done, stdout, cmd).
	p := &Process{}

	// Done returns nil channel (reads block forever, but must not panic).
	if p.Done() != nil {
		t.Fatal("expected nil done channel for zero-value Process")
	}

	// Result must not panic with nil stdout.
	res := p.Result()
	if res.Output != "" {
		t.Fatalf("expected empty output, got %q", res.Output)
	}
	if res.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", res.ExitCode)
	}

	// Kill must not panic with nil cmd.
	p.Kill()
}
