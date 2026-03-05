package session

import (
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestSessionCompact_Basic(t *testing.T) {
	sess := &Session{
		messages: make([]*schema.Message, 0, 8),
	}
	for i := 0; i < 10; i++ {
		sess.Append(&schema.Message{Role: schema.User, Content: "msg"})
	}

	summary := &schema.Message{Role: schema.Assistant, Content: "summary of first 7 messages"}
	sess.Compact(summary, 3) // keep last 3

	history := sess.History()
	if len(history) != 4 { // 1 summary + 3 recent
		t.Fatalf("History() len = %d, want 4", len(history))
	}
	if history[0].Content != "summary of first 7 messages" {
		t.Errorf("first message should be summary, got %q", history[0].Content)
	}
	if !sess.HasSummary() {
		t.Error("HasSummary() should return true after Compact")
	}
}

func TestSessionCompact_MsgCountUnchanged(t *testing.T) {
	sess := &Session{
		messages: make([]*schema.Message, 0, 8),
	}
	for i := 0; i < 5; i++ {
		sess.Append(&schema.Message{Role: schema.User, Content: "msg"})
	}
	countBefore := sess.MsgCount()

	summary := &schema.Message{Role: schema.Assistant, Content: "summary"}
	sess.Compact(summary, 2)

	if sess.MsgCount() != countBefore {
		t.Errorf("MsgCount changed from %d to %d after Compact", countBefore, sess.MsgCount())
	}
}

func TestSessionCompact_Double(t *testing.T) {
	sess := &Session{
		messages: make([]*schema.Message, 0, 8),
	}
	for i := 0; i < 10; i++ {
		sess.Append(&schema.Message{Role: schema.User, Content: "msg"})
	}

	// First compaction
	sess.Compact(&schema.Message{Role: schema.Assistant, Content: "summary1"}, 5)

	// Add more messages
	for i := 0; i < 5; i++ {
		sess.Append(&schema.Message{Role: schema.User, Content: "new msg"})
	}

	// Second compaction — old summary is part of "old" messages
	sess.Compact(&schema.Message{Role: schema.Assistant, Content: "summary2"}, 3)

	history := sess.History()
	if len(history) != 4 { // 1 summary + 3 recent
		t.Fatalf("History() len = %d, want 4", len(history))
	}
	if history[0].Content != "summary2" {
		t.Errorf("first message should be latest summary, got %q", history[0].Content)
	}
	if sess.compactVersion != 2 {
		t.Errorf("compactVersion = %d, want 2", sess.compactVersion)
	}
}

func TestSessionCompact_KeepCountExceedsMessages(t *testing.T) {
	sess := &Session{
		messages: make([]*schema.Message, 0, 8),
	}
	sess.Append(&schema.Message{Role: schema.User, Content: "only one"})

	summary := &schema.Message{Role: schema.Assistant, Content: "summary"}
	sess.Compact(summary, 10) // keepCount > len(messages)

	history := sess.History()
	// Should just prepend summary, keep all messages
	if len(history) != 2 { // 1 summary + 1 original
		t.Fatalf("History() len = %d, want 2", len(history))
	}
}

func TestSessionClear_ResetsSummary(t *testing.T) {
	sess := &Session{
		messages: make([]*schema.Message, 0, 8),
	}
	sess.Append(&schema.Message{Role: schema.User, Content: "msg"})
	sess.Compact(&schema.Message{Role: schema.Assistant, Content: "summary"}, 0)

	sess.Clear()

	if sess.HasSummary() {
		t.Error("HasSummary() should return false after Clear")
	}
	if len(sess.History()) != 0 {
		t.Errorf("History() should be empty after Clear, got %d", len(sess.History()))
	}
}

func TestSessionHasSummary_BeforeCompact(t *testing.T) {
	sess := &Session{
		messages: make([]*schema.Message, 0, 8),
	}
	if sess.HasSummary() {
		t.Error("HasSummary() should return false before any compaction")
	}
}
