package session

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cloudwego/eino/schema"

	"github.com/tgifai/friday/internal/channel"
)

const (
	sessKeyTpl = "agent:%s:%s:%s"
)

type Session struct {
	sessionKey string

	AgentID  string
	Channel  channel.Type
	UserID   string
	messages []*schema.Message

	createTime time.Time
	updateTime time.Time
	expireAt   time.Time

	msgCnt      atomic.Int64
	toolCallCnt atomic.Int64

	dirty   bool
	version uint64

	persistedMsgLen int
	appendSaveCnt   int

	mu sync.RWMutex
}

type sessionSnapshot struct {
	sessionKey string
	agentID    string
	channel    channel.Type
	userID     string

	createTime time.Time
	updateTime time.Time
	expireAt   time.Time

	msgCnt      int64
	toolCallCnt int64
	dirty       bool
	version     uint64

	messages        []*schema.Message
	persistedMsgLen int
	appendSaveCnt   int
}

func (s *Session) History() []*schema.Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	msgs := make([]*schema.Message, len(s.messages))
	copy(msgs, s.messages)
	return msgs
}

func (s *Session) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.messages = s.messages[:0]
	s.msgCnt.Store(0)
	s.toolCallCnt.Store(0)
	s.updateTime = time.Now()
	s.markMutationLocked()
}

func (s *Session) Append(msg *schema.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, msg)
	s.updateTime = time.Now()
	s.markMutationLocked()
}

func (s *Session) SetExpireAt(expireAt time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.expireAt.Equal(expireAt) {
		return
	}
	s.expireAt = expireAt
	s.markMutationLocked()
}

func (s *Session) MsgCount() int64 {
	return s.msgCnt.Load()
}

func (s *Session) ToolCallCount() int64 {
	return s.toolCallCnt.Load()
}

func (s *Session) UpdatedAt() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.updateTime
}

func (s *Session) IsExpired(now time.Time) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.expireAt.IsZero() {
		return false
	}
	return !s.expireAt.After(now)
}

func (s *Session) IncrMsgCnt() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.msgCnt.Add(1)
	s.updateTime = time.Now()
	s.markMutationLocked()
}

func (s *Session) IncrToolCallCnt() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.toolCallCnt.Add(1)
	s.updateTime = time.Now()
	s.markMutationLocked()
}

func (s *Session) snapshotForSave() sessionSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	msgs := make([]*schema.Message, len(s.messages))
	copy(msgs, s.messages)

	return sessionSnapshot{
		sessionKey:      s.sessionKey,
		agentID:         s.AgentID,
		channel:         s.Channel,
		userID:          s.UserID,
		createTime:      s.createTime,
		updateTime:      s.updateTime,
		expireAt:        s.expireAt,
		msgCnt:          s.msgCnt.Load(),
		toolCallCnt:     s.toolCallCnt.Load(),
		dirty:           s.dirty,
		version:         s.version,
		messages:        msgs,
		persistedMsgLen: s.persistedMsgLen,
		appendSaveCnt:   s.appendSaveCnt,
	}
}

func (s *Session) markPersisted(msgLen int, compacted bool, expectedVersion uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.persistedMsgLen = msgLen
	if compacted {
		s.appendSaveCnt = 0
	} else {
		s.appendSaveCnt++
	}
	if s.version == expectedVersion {
		s.dirty = false
	}
}

func (s *Session) markMutationLocked() {
	s.dirty = true
	s.version++
}

func GenerateKey(agentID string, channelType channel.Type, userID string) string {
	return fmt.Sprintf(sessKeyTpl, agentID, string(channelType), userID)
}

func ParseKey(sessionKey string) (agentID string, channelType channel.Type, userID string, err error) {
	parts := strings.Split(sessionKey, ":")
	if len(parts) != 4 || parts[0] != "agent" {
		return "", "", "", fmt.Errorf("invalid session key format: %s (expected agent:<agentId>:<channel>:<userId>)", sessionKey)
	}

	return parts[1], channel.Type(parts[2]), parts[3], nil
}
