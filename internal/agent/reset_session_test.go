package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"

	"github.com/tgifai/friday/internal/channel"
	"github.com/tgifai/friday/internal/config"
	"github.com/tgifai/friday/internal/consts"
)

func newTestAgent(t *testing.T) *Agent {
	t.Helper()
	workspace := t.TempDir()

	// Create required directories.
	os.MkdirAll(filepath.Join(workspace, "memory", "daily"), 0o755)
	os.MkdirAll(filepath.Join(workspace, "memory", "sessions"), 0o755)

	cfg := config.AgentConfig{
		ID:        "test-agent",
		Name:      "Test Agent",
		Workspace: workspace,
	}

	ag, err := NewAgent(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewAgent: %v", err)
	}
	return ag
}

func TestResetSession_Empty(t *testing.T) {
	ag := newTestAgent(t)

	msg := &channel.Message{
		ChannelType: channel.Telegram,
		ChannelID:   "tg-main",
		ChatID:      "chat-1",
	}

	reply, err := ag.ResetSession(context.Background(), msg)
	if err != nil {
		t.Fatalf("ResetSession: %v", err)
	}
	if !strings.Contains(reply, "already empty") {
		t.Errorf("expected 'already empty', got: %s", reply)
	}
}

func TestResetSession_WithMessages(t *testing.T) {
	ag := newTestAgent(t)

	msg := &channel.Message{
		ChannelType: channel.Telegram,
		ChannelID:   "tg-main",
		ChatID:      "chat-1",
	}

	// Pre-populate the session with messages.
	sess := ag.sess.GetOrCreateFor(msg.ChannelType, msg.ChannelID, msg.ChatID)
	sess.Append(&schema.Message{Role: schema.User, Content: "What is the weather?"})
	sess.Append(&schema.Message{Role: schema.Assistant, Content: "It's sunny today."})
	sess.Append(&schema.Message{Role: schema.User, Content: "Schedule a meeting."})
	ag.sess.Save(sess)

	reply, err := ag.ResetSession(context.Background(), msg)
	if err != nil {
		t.Fatalf("ResetSession: %v", err)
	}

	// Verify reply contains message count.
	if !strings.Contains(reply, "3 messages archived") {
		t.Errorf("expected '3 messages archived', got: %s", reply)
	}

	// Verify session is cleared.
	if sess.MsgCount() != 0 {
		t.Errorf("session should have 0 messages, got %d", sess.MsgCount())
	}
	if len(sess.History()) != 0 {
		t.Errorf("session history should be empty, got %d", len(sess.History()))
	}

	// Verify daily memory file was written.
	dailyPath := filepath.Join(ag.workspace, consts.DailyMemoryFile(time.Now()))
	content, err := os.ReadFile(dailyPath)
	if err != nil {
		t.Fatalf("read daily file: %v", err)
	}

	text := string(content)
	if !strings.Contains(text, "Session Reset") {
		t.Error("daily file should contain 'Session Reset' header")
	}
	if !strings.Contains(text, "What is the weather?") {
		t.Error("daily file should contain user message")
	}
	if !strings.Contains(text, "Schedule a meeting") {
		t.Error("daily file should contain second user message")
	}
	// Assistant messages should NOT be in the archive.
	if strings.Contains(text, "sunny today") {
		t.Error("daily file should NOT contain assistant messages")
	}
}

func TestResetSession_TruncatesLongMessages(t *testing.T) {
	ag := newTestAgent(t)

	msg := &channel.Message{
		ChannelType: channel.Telegram,
		ChannelID:   "tg-main",
		ChatID:      "chat-2",
	}

	sess := ag.sess.GetOrCreateFor(msg.ChannelType, msg.ChannelID, msg.ChatID)
	longMsg := strings.Repeat("a", 500)
	sess.Append(&schema.Message{Role: schema.User, Content: longMsg})
	ag.sess.Save(sess)

	_, err := ag.ResetSession(context.Background(), msg)
	if err != nil {
		t.Fatalf("ResetSession: %v", err)
	}

	dailyPath := filepath.Join(ag.workspace, consts.DailyMemoryFile(time.Now()))
	content, err := os.ReadFile(dailyPath)
	if err != nil {
		t.Fatalf("read daily file: %v", err)
	}

	if !strings.Contains(string(content), "...") {
		t.Error("long messages should be truncated with '...'")
	}
}
