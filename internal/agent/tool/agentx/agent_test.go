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

func TestAgentToolExecuteCreateUnknownBackend(t *testing.T) {
	tool := NewAgentTool("")
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

// ---------------------------------------------------------------------------
// Mock backend for integration tests
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Integration tests with mock backend
// ---------------------------------------------------------------------------

func TestAgentToolCreateWithMockBackend(t *testing.T) {
	tool := NewAgentTool("")
	tool.backends["mock"] = &testBackend{
		name:      "mock",
		available: true,
		runResult: &RunResult{
			CLISessionID: "cli-abc-123",
			Output:       "mock output from agent",
			ExitCode:     0,
		},
	}

	ctx := context.Background()
	res, err := tool.Execute(ctx, map[string]interface{}{
		"action":  "create",
		"backend": "mock",
		"prompt":  "write hello world",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m, ok := res.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map result, got %T", res)
	}

	// Status should be "completed" for a synchronous run.
	if m["status"] != StatusCompleted {
		t.Fatalf("expected status %q, got %q", StatusCompleted, m["status"])
	}

	// Result should contain the mock output.
	if m["result"] != "mock output from agent" {
		t.Fatalf("expected result %q, got %q", "mock output from agent", m["result"])
	}

	// cli_session_id should be set.
	if m["cli_session_id"] != "cli-abc-123" {
		t.Fatalf("expected cli_session_id %q, got %q", "cli-abc-123", m["cli_session_id"])
	}

	// session_id should be present.
	sessionID, ok := m["session_id"].(string)
	if !ok || sessionID == "" {
		t.Fatalf("expected non-empty session_id, got %v", m["session_id"])
	}

	// List should now show 1 session.
	listRes, err := tool.Execute(ctx, map[string]interface{}{"action": "list"})
	if err != nil {
		t.Fatalf("unexpected error listing sessions: %v", err)
	}
	listMap := listRes.(map[string]interface{})
	sessions := listMap["sessions"].([]map[string]interface{})
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0]["session_id"] != sessionID {
		t.Fatalf("expected session_id %q in list, got %q", sessionID, sessions[0]["session_id"])
	}
}

func TestAgentToolSendWithMockBackend(t *testing.T) {
	tool := NewAgentTool("")
	tool.backends["mock"] = &testBackend{
		name:      "mock",
		available: true,
		runResult: &RunResult{
			CLISessionID: "cli-abc-123",
			Output:       "initial output",
			ExitCode:     0,
		},
	}

	ctx := context.Background()

	// Step 1: Create a session.
	createRes, err := tool.Execute(ctx, map[string]interface{}{
		"action":  "create",
		"backend": "mock",
		"prompt":  "start session",
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	createMap := createRes.(map[string]interface{})
	sessionID := createMap["session_id"].(string)

	// Step 2: Update the mock to return follow-up output.
	tool.backends["mock"] = &testBackend{
		name:      "mock",
		available: true,
		runResult: &RunResult{
			CLISessionID: "cli-abc-123",
			Output:       "follow-up output",
			ExitCode:     0,
		},
	}

	// Step 3: Send a follow-up message.
	sendRes, err := tool.Execute(ctx, map[string]interface{}{
		"action":     "send",
		"session_id": sessionID,
		"prompt":     "continue the task",
	})
	if err != nil {
		t.Fatalf("send failed: %v", err)
	}

	sendMap, ok := sendRes.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map result, got %T", sendRes)
	}

	// Verify send returns the follow-up result.
	if sendMap["result"] != "follow-up output" {
		t.Fatalf("expected follow-up result %q, got %q", "follow-up output", sendMap["result"])
	}

	// Verify cli_session_id is preserved.
	if sendMap["cli_session_id"] != "cli-abc-123" {
		t.Fatalf("expected cli_session_id %q, got %q", "cli-abc-123", sendMap["cli_session_id"])
	}

	// Verify session_id is the same.
	if sendMap["session_id"] != sessionID {
		t.Fatalf("expected session_id %q, got %q", sessionID, sendMap["session_id"])
	}
}

func TestAgentToolDestroyWithMockBackend(t *testing.T) {
	tool := NewAgentTool("")
	tool.backends["mock"] = &testBackend{
		name:      "mock",
		available: true,
		runResult: &RunResult{
			CLISessionID: "cli-xyz-789",
			Output:       "created",
			ExitCode:     0,
		},
	}

	ctx := context.Background()

	// Step 1: Create a session.
	createRes, err := tool.Execute(ctx, map[string]interface{}{
		"action":  "create",
		"backend": "mock",
		"prompt":  "create something",
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	createMap := createRes.(map[string]interface{})
	sessionID := createMap["session_id"].(string)

	// Step 2: Destroy the session.
	destroyRes, err := tool.Execute(ctx, map[string]interface{}{
		"action":     "destroy",
		"session_id": sessionID,
	})
	if err != nil {
		t.Fatalf("destroy failed: %v", err)
	}

	destroyMap, ok := destroyRes.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map result, got %T", destroyRes)
	}

	// Verify destroy returns success.
	if destroyMap["success"] != true {
		t.Fatalf("expected success=true, got %v", destroyMap["success"])
	}
	if destroyMap["session_id"] != sessionID {
		t.Fatalf("expected session_id %q, got %q", sessionID, destroyMap["session_id"])
	}

	// Step 3: Verify status returns "not found" after destroy.
	_, err = tool.Execute(ctx, map[string]interface{}{
		"action":     "status",
		"session_id": sessionID,
	})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected 'not found' error after destroy, got %v", err)
	}
}

func TestAgentToolCreateAvailabilityCheck(t *testing.T) {
	tool := NewAgentTool("")
	tool.backends["mock"] = &testBackend{
		name:      "mock",
		available: false, // Backend is not available.
		runResult: &RunResult{
			CLISessionID: "cli-nope",
			Output:       "should not get here",
			ExitCode:     0,
		},
	}

	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"action":  "create",
		"backend": "mock",
		"prompt":  "try to create",
	})
	if err == nil {
		t.Fatal("expected error when backend is unavailable, got nil")
	}
	if !strings.Contains(err.Error(), "not found in PATH") {
		t.Fatalf("expected error containing 'not found in PATH', got %q", err.Error())
	}
}
