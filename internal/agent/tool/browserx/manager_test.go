package browserx

import (
	"testing"
)

func TestManagerGetSession(t *testing.T) {
	mgr := newBrowserManager()

	_, err := mgr.getSession("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent session")
	}

	sessions := mgr.listSessions()
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestManagerShutdownEmpty(t *testing.T) {
	mgr := newBrowserManager()
	mgr.shutdown()
}
