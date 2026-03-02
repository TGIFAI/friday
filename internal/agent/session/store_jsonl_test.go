package session

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestJSONLStore_CompactPersistence(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "sessions")
	store, err := newJSONLStore(storePath)
	if err != nil {
		t.Fatalf("newJSONLStore: %v", err)
	}
	ctx := context.Background()

	sess := &Session{
		SessionKey: "agent:test:telegram:main:user1",
		AgentID:    "test",
		messages:   make([]*schema.Message, 0, 8),
	}
	for i := 0; i < 6; i++ {
		sess.Append(&schema.Message{Role: schema.User, Content: "msg"})
		sess.Append(&schema.Message{Role: schema.Assistant, Content: "reply"})
	}
	if err := store.Save(ctx, sess); err != nil {
		t.Fatalf("initial Save: %v", err)
	}

	summary := &schema.Message{Role: schema.Assistant, Content: "summary of earlier conversation"}
	sess.Compact(summary, 4)
	if err := store.Save(ctx, sess); err != nil {
		t.Fatalf("Save after Compact: %v", err)
	}

	loaded, err := store.Load(ctx, "agent:test:telegram:main:user1")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded == nil {
		t.Fatal("Load returned nil")
	}
	if !loaded.HasSummary() {
		t.Error("loaded session should HasSummary()")
	}
	history := loaded.History()
	if len(history) != 5 {
		t.Fatalf("loaded History() len = %d, want 5", len(history))
	}
	if history[0].Content != "summary of earlier conversation" {
		t.Errorf("first message should be summary, got %q", history[0].Content)
	}
}

func TestJSONLStore_CompactThenAppend(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "sessions")
	store, err := newJSONLStore(storePath)
	if err != nil {
		t.Fatalf("newJSONLStore: %v", err)
	}
	ctx := context.Background()

	sess := &Session{
		SessionKey: "agent:test:telegram:main:user2",
		AgentID:    "test",
		messages:   make([]*schema.Message, 0, 8),
	}
	for i := 0; i < 4; i++ {
		sess.Append(&schema.Message{Role: schema.User, Content: "old"})
	}
	if err := store.Save(ctx, sess); err != nil {
		t.Fatalf("Save: %v", err)
	}

	sess.Compact(&schema.Message{Role: schema.Assistant, Content: "summary"}, 2)
	if err := store.Save(ctx, sess); err != nil {
		t.Fatalf("Save after compact: %v", err)
	}

	sess.Append(&schema.Message{Role: schema.User, Content: "new msg"})
	if err := store.Save(ctx, sess); err != nil {
		t.Fatalf("Save after append: %v", err)
	}

	loaded, err := store.Load(ctx, "agent:test:telegram:main:user2")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	history := loaded.History()
	if len(history) != 4 {
		t.Fatalf("loaded History() len = %d, want 4", len(history))
	}
	if history[len(history)-1].Content != "new msg" {
		t.Errorf("last message = %q, want 'new msg'", history[len(history)-1].Content)
	}
}

func TestJSONLStore_FullHistory_Preserved(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "sessions")
	store, err := newJSONLStore(storePath)
	if err != nil {
		t.Fatalf("newJSONLStore: %v", err)
	}
	ctx := context.Background()

	sess := &Session{
		SessionKey: "agent:test:telegram:main:user3",
		AgentID:    "test",
		messages:   make([]*schema.Message, 0, 8),
	}
	for i := 0; i < 4; i++ {
		sess.Append(&schema.Message{Role: schema.User, Content: "msg"})
	}
	if err := store.Save(ctx, sess); err != nil {
		t.Fatalf("Save: %v", err)
	}

	sess.Compact(&schema.Message{Role: schema.Assistant, Content: "summary"}, 2)
	if err := store.Save(ctx, sess); err != nil {
		t.Fatalf("Save after compact: %v", err)
	}

	js := store.(*jsonlStore)
	path := js.sessionFile("agent:test:telegram:main:user3")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(data), `"_type":"compact"`) {
		t.Error("JSONL should contain a compact record")
	}
}
