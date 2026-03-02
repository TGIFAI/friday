// internal/agent/session/compact.go
package session

import (
	"time"

	"github.com/cloudwego/eino/schema"
)

const CompactionSummaryKey = "compaction_summary"

// Compact replaces old messages with a summary, keeping the most recent
// keepCount messages. Only the in-memory view changes; the JSONL file
// retains full history for audit.
//
// If keepCount >= len(messages), the summary is prepended without removing
// any messages.
func (s *Session) Compact(summary *schema.Message, keepCount int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if summary.Extra == nil {
		summary.Extra = make(map[string]any)
	}
	summary.Extra[CompactionSummaryKey] = true

	msgLen := len(s.messages)
	if keepCount < 0 {
		keepCount = 0
	}
	if keepCount >= msgLen {
		keepCount = msgLen
	}

	// Keep only the tail.
	kept := make([]*schema.Message, keepCount)
	copy(kept, s.messages[msgLen-keepCount:])
	s.messages = kept

	s.summaryMsg = summary
	s.compactedAt = time.Now()
	s.compactVersion++

	// Reset persisted state so next Save does a full rewrite.
	s.persistedMsgLen = 0
	s.markMutationLocked()
}

// HasSummary reports whether this session has an active compaction summary.
func (s *Session) HasSummary() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.summaryMsg != nil
}
