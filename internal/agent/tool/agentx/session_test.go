package agentx

import "testing"

func TestSessionManagerCreate(t *testing.T) {
	sm := NewSessionManager(0)
	s := sm.Create("claude-code", "/tmp/work")

	if s.ID == "" {
		t.Fatal("expected non-empty session ID")
	}
	if s.Backend != "claude-code" {
		t.Fatalf("expected backend 'claude-code', got %q", s.Backend)
	}
	if s.Status != StatusRunning {
		t.Fatalf("expected status %q, got %q", StatusRunning, s.Status)
	}
	if s.WorkingDir != "/tmp/work" {
		t.Fatalf("expected working dir '/tmp/work', got %q", s.WorkingDir)
	}
	if s.CreatedAt.IsZero() {
		t.Fatal("expected non-zero CreatedAt")
	}
}

func TestSessionManagerGet(t *testing.T) {
	sm := NewSessionManager(0)
	s := sm.Create("claude-code", "/tmp/work")

	got, ok := sm.Get(s.ID)
	if !ok {
		t.Fatalf("expected Get(%q) to return true", s.ID)
	}
	if got.ID != s.ID {
		t.Fatalf("expected session ID %q, got %q", s.ID, got.ID)
	}

	_, ok = sm.Get("nonexistent-id")
	if ok {
		t.Fatal("expected Get for nonexistent ID to return false")
	}
}

func TestSessionManagerList(t *testing.T) {
	sm := NewSessionManager(0)
	sm.Create("claude-code", "/tmp/a")
	sm.Create("claude-code", "/tmp/b")

	list := sm.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(list))
	}
}

func TestSessionManagerDestroy(t *testing.T) {
	sm := NewSessionManager(0)
	s := sm.Create("claude-code", "/tmp/work")

	sm.Destroy(s.ID)

	_, ok := sm.Get(s.ID)
	if ok {
		t.Fatal("expected session to be gone after Destroy")
	}

	// Destroying a nonexistent session should not panic.
	sm.Destroy("nonexistent-id")
}

func TestSessionManagerDestroyKillsProcess(t *testing.T) {
	sm := NewSessionManager(0)
	s := sm.Create("claude-code", "/tmp/work")
	// Attach a zero-value Process (Kill is safe on nil cmd).
	s.process = &Process{}

	// Must not panic even though cmd is nil.
	sm.Destroy(s.ID)

	_, ok := sm.Get(s.ID)
	if ok {
		t.Fatal("expected session to be gone after Destroy")
	}
}

func TestSessionManagerMaxSessions(t *testing.T) {
	sm := NewSessionManager(2)

	_, err := sm.CreateWithLimit("claude-code", "/tmp/a")
	if err != nil {
		t.Fatalf("unexpected error on first create: %v", err)
	}
	_, err = sm.CreateWithLimit("claude-code", "/tmp/b")
	if err != nil {
		t.Fatalf("unexpected error on second create: %v", err)
	}

	_, err = sm.CreateWithLimit("claude-code", "/tmp/c")
	if err == nil {
		t.Fatal("expected error when exceeding max sessions, got nil")
	}
}
