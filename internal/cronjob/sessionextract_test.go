package cronjob

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestExtractUserMessages_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	excerpts, err := ExtractUserMessages(dir, time.Now(), 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(excerpts) != 0 {
		t.Fatalf("expected 0 excerpts, got %d", len(excerpts))
	}
}

func TestExtractUserMessages_MissingDir(t *testing.T) {
	excerpts, err := ExtractUserMessages("/nonexistent/path", time.Now(), 20)
	if err != nil {
		t.Fatalf("unexpected error for missing dir: %v", err)
	}
	if len(excerpts) != 0 {
		t.Fatalf("expected 0 excerpts, got %d", len(excerpts))
	}
}

func TestExtractUserMessages_FiltersCronSessions(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	updatedAt := now.Format(time.RFC3339)

	// Cron session should be skipped.
	cronContent := `{"_type":"meta","session_key":"cron:__heartbeat__:agent-1","updated_at":"` + updatedAt + `","format":"friday-session-jsonl","schema":1}
{"_type":"msg","msg":{"role":"user","content":"heartbeat check"}}
`
	os.WriteFile(filepath.Join(dir, "cron-session.jsonl"), []byte(cronContent), 0o644)

	excerpts, err := ExtractUserMessages(dir, now, 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(excerpts) != 0 {
		t.Fatalf("expected 0 excerpts (cron filtered), got %d", len(excerpts))
	}
}

func TestExtractUserMessages_ExtractsUserOnly(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	updatedAt := now.Format(time.RFC3339)

	content := `{"_type":"meta","session_key":"agent:default:telegram:main:user1","updated_at":"` + updatedAt + `","format":"friday-session-jsonl","schema":1}
{"_type":"msg","msg":{"role":"user","content":"Hello Friday"}}
{"_type":"msg","msg":{"role":"assistant","content":"Hello! How can I help?"}}
{"_type":"msg","msg":{"role":"user","content":"What time is it?"}}
{"_type":"msg","msg":{"role":"assistant","content":"It's 3pm."}}
`
	os.WriteFile(filepath.Join(dir, "user-session.jsonl"), []byte(content), 0o644)

	excerpts, err := ExtractUserMessages(dir, now, 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(excerpts) != 1 {
		t.Fatalf("expected 1 excerpt, got %d", len(excerpts))
	}
	if len(excerpts[0].Messages) != 2 {
		t.Fatalf("expected 2 user messages, got %d", len(excerpts[0].Messages))
	}
	if excerpts[0].Messages[0] != "Hello Friday" {
		t.Errorf("first message = %q, want Hello Friday", excerpts[0].Messages[0])
	}
	if excerpts[0].Messages[1] != "What time is it?" {
		t.Errorf("second message = %q, want What time is it?", excerpts[0].Messages[1])
	}
}

func TestExtractUserMessages_RespectsMaxPerSession(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	updatedAt := now.Format(time.RFC3339)

	content := `{"_type":"meta","session_key":"agent:default:telegram:main:user1","updated_at":"` + updatedAt + `","format":"friday-session-jsonl","schema":1}
{"_type":"msg","msg":{"role":"user","content":"msg1"}}
{"_type":"msg","msg":{"role":"user","content":"msg2"}}
{"_type":"msg","msg":{"role":"user","content":"msg3"}}
{"_type":"msg","msg":{"role":"user","content":"msg4"}}
{"_type":"msg","msg":{"role":"user","content":"msg5"}}
`
	os.WriteFile(filepath.Join(dir, "multi-msg.jsonl"), []byte(content), 0o644)

	excerpts, err := ExtractUserMessages(dir, now, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(excerpts) != 1 {
		t.Fatalf("expected 1 excerpt, got %d", len(excerpts))
	}
	if len(excerpts[0].Messages) != 3 {
		t.Fatalf("expected 3 messages (maxPerSession=3), got %d", len(excerpts[0].Messages))
	}
}

func TestExtractUserMessages_SkipsOldSessions(t *testing.T) {
	dir := t.TempDir()
	// Session from 2 days ago.
	twoDaysAgo := time.Now().AddDate(0, 0, -2)
	updatedAt := twoDaysAgo.Format(time.RFC3339)

	content := `{"_type":"meta","session_key":"agent:default:telegram:main:user1","updated_at":"` + updatedAt + `","format":"friday-session-jsonl","schema":1}
{"_type":"msg","msg":{"role":"user","content":"old message"}}
`
	os.WriteFile(filepath.Join(dir, "old-session.jsonl"), []byte(content), 0o644)

	// Query for today — should find nothing.
	excerpts, err := ExtractUserMessages(dir, time.Now(), 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(excerpts) != 0 {
		t.Fatalf("expected 0 excerpts (old session), got %d", len(excerpts))
	}
}

func TestExtractUserMessages_MultipleMetaRecords(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	oldDate := now.AddDate(0, 0, -3).Format(time.RFC3339)
	newDate := now.Format(time.RFC3339)

	// Simulate append-save: first meta has old date, last meta has today's date.
	// The last meta record should win.
	content := `{"_type":"meta","session_key":"agent:default:telegram:main:user1","updated_at":"` + oldDate + `","format":"friday-session-jsonl","schema":1}
{"_type":"msg","msg":{"role":"user","content":"first message"}}
{"_type":"msg","msg":{"role":"assistant","content":"response"}}
{"_type":"meta","session_key":"agent:default:telegram:main:user1","updated_at":"` + newDate + `","format":"friday-session-jsonl","schema":1}
{"_type":"msg","msg":{"role":"user","content":"second message"}}
`
	os.WriteFile(filepath.Join(dir, "multi-meta.jsonl"), []byte(content), 0o644)

	excerpts, err := ExtractUserMessages(dir, now, 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(excerpts) != 1 {
		t.Fatalf("expected 1 excerpt (last meta is in range), got %d", len(excerpts))
	}
	// Both user messages should be collected regardless of which meta they follow.
	if len(excerpts[0].Messages) != 2 {
		t.Fatalf("expected 2 user messages, got %d", len(excerpts[0].Messages))
	}
	if excerpts[0].Messages[0] != "first message" {
		t.Errorf("msg[0] = %q, want first message", excerpts[0].Messages[0])
	}
	if excerpts[0].Messages[1] != "second message" {
		t.Errorf("msg[1] = %q, want second message", excerpts[0].Messages[1])
	}
}
