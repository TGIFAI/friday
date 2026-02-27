package cronjob

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewFlushJob(t *testing.T) {
	job := NewFlushJob("agent-1", "/workspace")

	if !IsFlushJob(job.ID) {
		t.Errorf("job ID %q should be detected as flush", job.ID)
	}
	if job.AgentID != "agent-1" {
		t.Errorf("agent ID = %q, want agent-1", job.AgentID)
	}
	if job.SessionTarget != SessionIsolated {
		t.Errorf("session target = %q, want isolated", job.SessionTarget)
	}
	if job.ScheduleType != ScheduleCron {
		t.Errorf("schedule type = %q, want cron", job.ScheduleType)
	}
	if job.Schedule != "45 1 * * *" {
		t.Errorf("schedule = %q, want 45 1 * * *", job.Schedule)
	}
	if !job.Enabled {
		t.Error("job should be enabled")
	}
}

func TestIsFlushJob(t *testing.T) {
	tests := []struct {
		id   string
		want bool
	}{
		{"regular-job", false},
		{FlushJobID + ":agent-1", true},
		{FlushJobID, true},
		{HeartbeatJobID + ":agent-1", false},
		{CompactJobID + ":agent-1", false},
	}

	for _, tt := range tests {
		if got := IsFlushJob(tt.id); got != tt.want {
			t.Errorf("IsFlushJob(%q) = %v, want %v", tt.id, got, tt.want)
		}
	}
}

func TestBuildFlushPrompt_NoActivity(t *testing.T) {
	workspace := t.TempDir()
	os.MkdirAll(filepath.Join(workspace, "memory", "sessions"), 0o755)
	os.MkdirAll(filepath.Join(workspace, "memory", "daily"), 0o755)

	_, hasWork := BuildFlushPrompt(workspace, time.Now())
	if hasWork {
		t.Fatal("expected no work when no session activity")
	}
}

func TestBuildFlushPrompt_WithActivity(t *testing.T) {
	workspace := t.TempDir()
	sessDir := filepath.Join(workspace, "memory", "sessions")
	os.MkdirAll(sessDir, 0o755)
	os.MkdirAll(filepath.Join(workspace, "memory", "daily"), 0o755)

	now := time.Now()
	updatedAt := now.Format(time.RFC3339)

	sessionContent := `{"_type":"meta","session_key":"agent:default:telegram:main:user1","updated_at":"` + updatedAt + `","format":"friday-session-jsonl","schema":1}
{"_type":"msg","msg":{"role":"user","content":"Please schedule a meeting for tomorrow."}}
{"_type":"msg","msg":{"role":"assistant","content":"Done, meeting scheduled."}}
`
	os.WriteFile(filepath.Join(sessDir, "test-session.jsonl"), []byte(sessionContent), 0o644)

	prompt, hasWork := BuildFlushPrompt(workspace, now)
	if !hasWork {
		t.Fatal("expected work when session activity exists")
	}
	if !strings.Contains(prompt, "Memory Flush") {
		t.Error("prompt should contain Memory Flush header")
	}
	if !strings.Contains(prompt, "schedule a meeting") {
		t.Error("prompt should contain user message content")
	}
	if !strings.Contains(prompt, "Task 1: Update daily file") {
		t.Error("prompt should contain Task 1 instructions")
	}
	if !strings.Contains(prompt, "Task 2: Extract persistent facts") {
		t.Error("prompt should contain Task 2 instructions")
	}
}

func TestBuildFlushPrompt_WithExistingDailyFile(t *testing.T) {
	workspace := t.TempDir()
	sessDir := filepath.Join(workspace, "memory", "sessions")
	dailyDir := filepath.Join(workspace, "memory", "daily")
	os.MkdirAll(sessDir, 0o755)
	os.MkdirAll(dailyDir, 0o755)

	now := time.Now()
	updatedAt := now.Format(time.RFC3339)
	dateStr := now.Format("2006-01-02")

	// Create session activity.
	sessionContent := `{"_type":"meta","session_key":"agent:default:telegram:main:user1","updated_at":"` + updatedAt + `","format":"friday-session-jsonl","schema":1}
{"_type":"msg","msg":{"role":"user","content":"Hello world"}}
`
	os.WriteFile(filepath.Join(sessDir, "test.jsonl"), []byte(sessionContent), 0o644)

	// Create existing daily file.
	dailyPath := filepath.Join(dailyDir, dateStr+".md")
	os.WriteFile(dailyPath, []byte("- Had a meeting at 10am\n"), 0o644)

	prompt, hasWork := BuildFlushPrompt(workspace, now)
	if !hasWork {
		t.Fatal("expected work")
	}
	if !strings.Contains(prompt, "Existing Daily File") {
		t.Error("prompt should reference existing daily file")
	}
	if !strings.Contains(prompt, "Had a meeting") {
		t.Error("prompt should contain existing daily content")
	}
}

func TestBuildFlushPrompt_SkipsCronSessions(t *testing.T) {
	workspace := t.TempDir()
	sessDir := filepath.Join(workspace, "memory", "sessions")
	os.MkdirAll(sessDir, 0o755)

	now := time.Now()
	updatedAt := now.Format(time.RFC3339)

	// Only cron session activity — should be filtered out.
	sessionContent := `{"_type":"meta","session_key":"cron:__heartbeat__:default","updated_at":"` + updatedAt + `","format":"friday-session-jsonl","schema":1}
{"_type":"msg","msg":{"role":"user","content":"heartbeat check"}}
`
	os.WriteFile(filepath.Join(sessDir, "cron.jsonl"), []byte(sessionContent), 0o644)

	_, hasWork := BuildFlushPrompt(workspace, now)
	if hasWork {
		t.Fatal("expected no work when only cron sessions exist")
	}
}
