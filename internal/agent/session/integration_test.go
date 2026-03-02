package session

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/cloudwego/eino/schema"
)

// TestCompactionLifecycle tests the full lifecycle:
// create → populate → compact → save → load → verify → append → save → load → verify
func TestCompactionLifecycle(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "sessions")
	store, err := newJSONLStore(storePath)
	if err != nil {
		t.Fatalf("newJSONLStore: %v", err)
	}
	ctx := context.Background()
	sessKey := "agent:test:telegram:main:lifecycle"

	// 1. Create and populate.
	sess := &Session{
		SessionKey: sessKey,
		AgentID:    "test",
		messages:   make([]*schema.Message, 0, 16),
	}
	for i := 0; i < 20; i++ {
		sess.Append(&schema.Message{Role: schema.User, Content: "question"})
		sess.Append(&schema.Message{Role: schema.Assistant, Content: "answer"})
	}
	if err := store.Save(ctx, sess); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if sess.MsgCount() != 40 {
		t.Fatalf("MsgCount = %d, want 40", sess.MsgCount())
	}

	// 2. Compact: keep last 6 messages.
	summary := &schema.Message{Role: schema.Assistant, Content: "Summary: 17 Q&A rounds about various topics"}
	sess.Compact(summary, 6)
	if err := store.Save(ctx, sess); err != nil {
		t.Fatalf("Save after compact: %v", err)
	}

	// 3. Load and verify.
	loaded, err := store.Load(ctx, sessKey)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	history := loaded.History()
	if len(history) != 7 { // 1 summary + 6 kept
		t.Fatalf("loaded History() len = %d, want 7", len(history))
	}
	if !loaded.HasSummary() {
		t.Error("loaded session should HasSummary()")
	}

	// 4. Append new messages after loading.
	loaded.Append(&schema.Message{Role: schema.User, Content: "new question"})
	loaded.Append(&schema.Message{Role: schema.Assistant, Content: "new answer"})
	if err := store.Save(ctx, loaded); err != nil {
		t.Fatalf("Save after append: %v", err)
	}

	// 5. Load again and verify.
	loaded2, err := store.Load(ctx, sessKey)
	if err != nil {
		t.Fatalf("Load2: %v", err)
	}
	history2 := loaded2.History()
	if len(history2) != 9 { // 1 summary + 6 kept + 2 new
		t.Fatalf("loaded2 History() len = %d, want 9", len(history2))
	}
	if history2[len(history2)-1].Content != "new answer" {
		t.Errorf("last message = %q, want 'new answer'", history2[len(history2)-1].Content)
	}

	// 6. Verify MsgCount survived.
	if loaded2.MsgCount() != 42 { // 40 original + 2 new
		t.Errorf("MsgCount = %d, want 42", loaded2.MsgCount())
	}
}

// TestEstimateTokens_ThresholdDetection verifies the estimation is reasonable
// for deciding when compaction should trigger.
func TestEstimateTokens_ThresholdDetection(t *testing.T) {
	// Build a large message list that should clearly exceed 100K tokens.
	msgs := make([]*schema.Message, 0, 1000)
	// Each message: 500 chars of 'x' → 500 bytes → 125 tokens. 1000 messages = ~125K tokens.
	longContent := make([]byte, 500)
	for i := range longContent {
		longContent[i] = 'x'
	}
	contentStr := string(longContent)
	for i := 0; i < 1000; i++ {
		msgs = append(msgs, &schema.Message{Role: schema.User, Content: contentStr})
	}

	estimated := EstimateTokens(msgs)
	if estimated < 100000 {
		t.Errorf("EstimateTokens = %d, expected > 100000 for 500KB of content", estimated)
	}
}
