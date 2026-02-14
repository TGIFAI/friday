package session

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/schema"

	"github.com/tgifai/friday/internal/channel"
)

const sessKeyTpl = "agent:%s:%s:%s:%s"

type Session struct {
	SessionKey string

	AgentID   string
	Channel   channel.Type
	ChannelID string
	ChatID    string
	messages []*schema.Message

	createTime time.Time
	updateTime time.Time
	expireAt   time.Time

	msgCnt      int64
	toolCallCnt int64

	dirty   bool
	version uint64

	persistedMsgLen int
	appendSaveCnt   int

	mu sync.RWMutex
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
	s.msgCnt = 0
	s.toolCallCnt = 0
	s.updateTime = time.Now()
	s.markMutationLocked()
}

func (s *Session) Append(msg *schema.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, msg)
	s.msgCnt++
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
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.msgCnt
}

func (s *Session) ToolCallCount() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.toolCallCnt
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

func (s *Session) markMutationLocked() {
	s.dirty = true
	s.version++
}

func GenerateKey(agentID string, channelType channel.Type, channelID string, chatID string) string {
	return fmt.Sprintf(sessKeyTpl, agentID, string(channelType), channelID, chatID)
}

func ParseKey(sessionKey string) (agentID string, channelType channel.Type, channelID string, chatID string, err error) {
	parts := strings.Split(sessionKey, ":")
	if len(parts) != 5 || parts[0] != "agent" {
		return "", "", "", "", fmt.Errorf("invalid session key format: %s (expected agent:<agentId>:<channel>:<channelId>:<chatId>)", sessionKey)
	}

	return parts[1], channel.Type(parts[2]), parts[3], parts[4], nil
}
