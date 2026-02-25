package agent

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"

	"github.com/tgifai/friday/internal/channel"
	"github.com/tgifai/friday/internal/config"
)

func newTestAgentWithEnqueue(t *testing.T, consolidateEvery int) (*Agent, *atomic.Int32) {
	t.Helper()
	workspace := t.TempDir()

	os.MkdirAll(filepath.Join(workspace, "memory", "daily"), 0o755)
	os.MkdirAll(filepath.Join(workspace, "memory", "sessions"), 0o755)

	cfg := config.AgentConfig{
		ID:        "test-agent",
		Name:      "Test Agent",
		Workspace: workspace,
		Session: config.SessionConfig{
			ConsolidateEvery: consolidateEvery,
			FlushCooldown:    "1s", // short cooldown for testing
		},
	}

	ag, err := NewAgent(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewAgent: %v", err)
	}

	// Create a session file with user messages so BuildFlushPrompt finds activity.
	sessDir := filepath.Join(workspace, "memory", "sessions")
	now := time.Now().Format(time.RFC3339)
	sessionContent := `{"_type":"meta","session_key":"agent:test-agent:telegram:main:user1","updated_at":"` + now + `","format":"friday-session-jsonl","schema":1}
{"_type":"msg","msg":{"role":"user","content":"test message for flush"}}
`
	os.WriteFile(filepath.Join(sessDir, "test-session.jsonl"), []byte(sessionContent), 0o644)

	var called atomic.Int32
	ag.SetEnqueue(func(_ context.Context, _ *channel.Message) error {
		called.Add(1)
		return nil
	})

	return ag, &called
}

func TestMaybeEnqueueFlush_BelowThreshold(t *testing.T) {
	ag, called := newTestAgentWithEnqueue(t, 10)

	msg := &channel.Message{
		ChannelType: channel.Telegram,
		ChannelID:   "tg-main",
		ChatID:      "chat-1",
	}
	sess := ag.sess.GetOrCreateFor(msg.ChannelType, msg.ChannelID, msg.ChatID)

	// Add 5 messages (below threshold of 10).
	for i := 0; i < 5; i++ {
		sess.Append(&schema.Message{Role: schema.User, Content: "msg"})
	}

	ag.maybeEnqueueFlush(context.Background(), sess)

	// Give goroutine time to run (if it were triggered).
	time.Sleep(50 * time.Millisecond)

	if called.Load() != 0 {
		t.Error("enqueue should NOT be called below threshold")
	}
}

func TestMaybeEnqueueFlush_AtThreshold(t *testing.T) {
	ag, called := newTestAgentWithEnqueue(t, 5)

	msg := &channel.Message{
		ChannelType: channel.Telegram,
		ChannelID:   "tg-main",
		ChatID:      "chat-1",
	}
	sess := ag.sess.GetOrCreateFor(msg.ChannelType, msg.ChannelID, msg.ChatID)

	// Add exactly 5 messages (at threshold).
	for i := 0; i < 5; i++ {
		sess.Append(&schema.Message{Role: schema.User, Content: "msg"})
	}

	ag.maybeEnqueueFlush(context.Background(), sess)

	// Give goroutine time to run.
	time.Sleep(100 * time.Millisecond)

	if called.Load() != 1 {
		t.Errorf("enqueue should be called once at threshold, got %d", called.Load())
	}

	// Verify metadata was set.
	if sess.GetMeta("last_flush_at") == "" {
		t.Error("last_flush_at metadata should be set")
	}
	if sess.GetMeta("flush_at_msg_cnt") != "5" {
		t.Errorf("flush_at_msg_cnt = %q, want 5", sess.GetMeta("flush_at_msg_cnt"))
	}
}

func TestMaybeEnqueueFlush_Cooldown(t *testing.T) {
	ag, called := newTestAgentWithEnqueue(t, 5)

	msg := &channel.Message{
		ChannelType: channel.Telegram,
		ChannelID:   "tg-main",
		ChatID:      "chat-1",
	}
	sess := ag.sess.GetOrCreateFor(msg.ChannelType, msg.ChannelID, msg.ChatID)

	// Simulate a recent flush.
	sess.SetMeta("last_flush_at", time.Now().Format(time.RFC3339))
	sess.SetMeta("flush_at_msg_cnt", "0")

	// Add messages past threshold.
	for i := 0; i < 10; i++ {
		sess.Append(&schema.Message{Role: schema.User, Content: "msg"})
	}

	ag.maybeEnqueueFlush(context.Background(), sess)

	time.Sleep(50 * time.Millisecond)

	if called.Load() != 0 {
		t.Error("enqueue should NOT be called during cooldown")
	}
}

func TestMaybeEnqueueFlush_NoEnqueueFunc(t *testing.T) {
	workspace := t.TempDir()
	os.MkdirAll(filepath.Join(workspace, "memory", "sessions"), 0o755)

	cfg := config.AgentConfig{
		ID:        "test-agent",
		Name:      "Test Agent",
		Workspace: workspace,
		Session: config.SessionConfig{
			ConsolidateEvery: 5,
		},
	}

	ag, err := NewAgent(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewAgent: %v", err)
	}
	// Do NOT set enqueue func.

	msg := &channel.Message{
		ChannelType: channel.Telegram,
		ChannelID:   "tg-main",
		ChatID:      "chat-1",
	}
	sess := ag.sess.GetOrCreateFor(msg.ChannelType, msg.ChannelID, msg.ChatID)
	for i := 0; i < 10; i++ {
		sess.Append(&schema.Message{Role: schema.User, Content: "msg"})
	}

	// Should not panic.
	ag.maybeEnqueueFlush(context.Background(), sess)
}

func TestMaybeEnqueueFlush_DoubleThreshold(t *testing.T) {
	ag, called := newTestAgentWithEnqueue(t, 5)
	ag.flushCooldown = 0 // disable cooldown for this test

	msg := &channel.Message{
		ChannelType: channel.Telegram,
		ChannelID:   "tg-main",
		ChatID:      "chat-1",
	}
	sess := ag.sess.GetOrCreateFor(msg.ChannelType, msg.ChannelID, msg.ChatID)

	// First batch: 5 messages.
	for i := 0; i < 5; i++ {
		sess.Append(&schema.Message{Role: schema.User, Content: "msg"})
	}
	ag.maybeEnqueueFlush(context.Background(), sess)
	time.Sleep(50 * time.Millisecond)

	if called.Load() != 1 {
		t.Fatalf("first flush: expected 1 call, got %d", called.Load())
	}

	// Second batch: 5 more messages (total 10).
	for i := 0; i < 5; i++ {
		sess.Append(&schema.Message{Role: schema.User, Content: "msg"})
	}

	// Update flush_at_msg_cnt to current count to simulate the first flush.
	sess.SetMeta("flush_at_msg_cnt", strconv.FormatInt(5, 10))
	sess.SetMeta("last_flush_at", time.Now().Add(-time.Hour).Format(time.RFC3339)) // expired cooldown

	ag.maybeEnqueueFlush(context.Background(), sess)
	time.Sleep(50 * time.Millisecond)

	if called.Load() != 2 {
		t.Errorf("second flush: expected 2 calls total, got %d", called.Load())
	}
}
