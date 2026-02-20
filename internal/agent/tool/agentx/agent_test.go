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
