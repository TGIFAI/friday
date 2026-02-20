package cronjob

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestNewCompactJob(t *testing.T) {
	job := NewCompactJob("agent-1", "/workspace")

	if !IsCompactJob(job.ID) {
		t.Errorf("job ID %q should be detected as compact", job.ID)
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
	if job.Schedule != "0 2 * * *" {
		t.Errorf("schedule = %q, want 0 2 * * *", job.Schedule)
	}
}

func TestIsCompactJob(t *testing.T) {
	if IsCompactJob("regular-job") {
		t.Error("regular job should not be compact")
	}
	if !IsCompactJob(CompactJobID + ":agent-1") {
		t.Error("compact job should be detected")
	}
}

func TestBuildCompactPrompt_NoData(t *testing.T) {
	workspace := t.TempDir()
	os.MkdirAll(filepath.Join(workspace, "memory", "sessions"), 0o755)
	os.MkdirAll(filepath.Join(workspace, "memory", "daily"), 0o755)

	_, hasWork := BuildCompactPrompt(workspace, time.Now())
	if hasWork {
		t.Fatal("expected no work when no daily file and no sessions")
	}
}

func TestBuildCompactPrompt_WithDailyFile(t *testing.T) {
	workspace := t.TempDir()
	os.MkdirAll(filepath.Join(workspace, "memory", "sessions"), 0o755)
	dailyDir := filepath.Join(workspace, "memory", "daily")
	os.MkdirAll(dailyDir, 0o755)

	now := time.Now()
	yesterday := now.AddDate(0, 0, -1)
	dailyFile := filepath.Join(dailyDir, yesterday.Format("2006-01-02")+".md")
	os.WriteFile(dailyFile, []byte("## Events\n- Had a meeting about project X\n- Deployed v2.1\n"), 0o644)

	prompt, hasWork := BuildCompactPrompt(workspace, now)
	if !hasWork {
		t.Fatal("expected work when daily file exists")
	}
	if !strings.Contains(prompt, "Memory Compaction") {
		t.Error("prompt should contain Memory Compaction header")
	}
	if !strings.Contains(prompt, "Had a meeting about project X") {
		t.Error("prompt should contain daily file content")
	}
	if !strings.Contains(prompt, "Task 1: Condense daily file") {
		t.Error("prompt should contain Task 1 instructions")
	}
	if !strings.Contains(prompt, "Task 2: Extract persistent facts") {
		t.Error("prompt should contain Task 2 instructions")
	}
}

func TestBuildCompactPrompt_WithSessionActivity(t *testing.T) {
	workspace := t.TempDir()
	sessDir := filepath.Join(workspace, "memory", "sessions")
	os.MkdirAll(sessDir, 0o755)
	os.MkdirAll(filepath.Join(workspace, "memory", "daily"), 0o755)

	now := time.Now()
	yesterday := now.AddDate(0, 0, -1)
	updatedAt := yesterday.Format(time.RFC3339)

	// Create a session file with user messages from yesterday.
	sessionContent := `{"_type":"meta","session_key":"agent:default:telegram:main:user1","updated_at":"` + updatedAt + `","format":"friday-session-jsonl","schema":1}
{"_type":"msg","msg":{"role":"user","content":"What is the weather today?"}}
{"_type":"msg","msg":{"role":"assistant","content":"It's sunny and 22°C."}}
{"_type":"msg","msg":{"role":"user","content":"Remind me to buy groceries."}}
`
	os.WriteFile(filepath.Join(sessDir, "test-session.jsonl"), []byte(sessionContent), 0o644)

	prompt, hasWork := BuildCompactPrompt(workspace, now)
	if !hasWork {
		t.Fatal("expected work when session activity exists")
	}
	if !strings.Contains(prompt, "Session Activity Summary") {
		t.Error("prompt should contain session activity section")
	}
	if !strings.Contains(prompt, "What is the weather today?") {
		t.Error("prompt should contain user messages")
	}
	if !strings.Contains(prompt, "Remind me to buy groceries") {
		t.Error("prompt should contain user messages")
	}
}

func TestBuildCompactPrompt_WithFixedDate(t *testing.T) {
	workspace := t.TempDir()
	os.MkdirAll(filepath.Join(workspace, "memory", "sessions"), 0o755)
	dailyDir := filepath.Join(workspace, "memory", "daily")
	os.MkdirAll(dailyDir, 0o755)

	// Use a fixed "now" to verify the date is threaded consistently.
	now := time.Date(2026, 3, 15, 2, 0, 0, 0, time.Local)
	dailyFile := filepath.Join(dailyDir, "2026-03-14.md")
	os.WriteFile(dailyFile, []byte("- deployed v3"), 0o644)

	prompt, hasWork := BuildCompactPrompt(workspace, now)
	if !hasWork {
		t.Fatal("expected work")
	}
	if !strings.Contains(prompt, "2026-03-14") {
		t.Error("prompt should reference yesterday's date 2026-03-14")
	}
}

func TestBuildCompactPrompt_UTF8Truncation(t *testing.T) {
	workspace := t.TempDir()
	sessDir := filepath.Join(workspace, "memory", "sessions")
	os.MkdirAll(sessDir, 0o755)
	os.MkdirAll(filepath.Join(workspace, "memory", "daily"), 0o755)

	now := time.Now()
	yesterday := now.AddDate(0, 0, -1)
	updatedAt := yesterday.Format(time.RFC3339)

	// Build a message with >500 CJK characters (each 3 bytes in UTF-8).
	longCJK := strings.Repeat("\u4e16", 600) // 600 copies of '世'

	sessionContent := `{"_type":"meta","session_key":"agent:default:telegram:main:user1","updated_at":"` + updatedAt + `","format":"friday-session-jsonl","schema":1}
{"_type":"msg","msg":{"role":"user","content":"` + longCJK + `"}}
`
	os.WriteFile(filepath.Join(sessDir, "cjk-session.jsonl"), []byte(sessionContent), 0o644)

	prompt, hasWork := BuildCompactPrompt(workspace, now)
	if !hasWork {
		t.Fatal("expected work")
	}

	// The prompt must be valid UTF-8 after truncation.
	if !utf8.ValidString(prompt) {
		t.Fatal("prompt contains invalid UTF-8 after truncation")
	}

	// The truncated message should end with "..." and not include all 600 chars.
	if strings.Contains(prompt, longCJK) {
		t.Error("expected message to be truncated, but full content found")
	}
	if !strings.Contains(prompt, "...") {
		t.Error("expected truncation marker '...' in prompt")
	}
}
