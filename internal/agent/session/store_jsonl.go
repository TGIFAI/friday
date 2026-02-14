package session

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/cloudwego/eino/schema"

	"github.com/tgifai/friday/internal/channel"
)

const (
	jsonlFormat           = "friday-session-jsonl"
	jsonlSchema           = 1
	defaultCompactEvery   = 20
	defaultCompactMaxSize = 4 * 1024 * 1024
)

type jsonlStore struct {
	root           string
	compactEvery   int
	compactMaxSize int64
	sessionLocks   sync.Map
}

type jsonlRecordHeader struct {
	Type string `json:"_type"`
}

type jsonlMetadataRecord struct {
	Type        string    `json:"_type"`
	SessionKey  string    `json:"session_key"`
	AgentID     string    `json:"agent_id,omitempty"`
	Channel     string    `json:"channel,omitempty"`
	ChannelID   string    `json:"channel_id,omitempty"`
	ChatID      string    `json:"chat_id,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	ExpireAt    time.Time `json:"expire_at,omitempty"`
	MsgCount    int64     `json:"msg_count"`
	ToolCallCnt int64     `json:"tool_call_count"`
	Format      string    `json:"format"`
	Schema      int       `json:"schema"`
}

type jsonlMessageRecord struct {
	Type    string          `json:"_type"`
	Message *schema.Message `json:"msg"`
}

func NewJSONLManager(agentID string, workspace string) (*Manager, error) {
	if workspace == "" {
		return nil, fmt.Errorf("workspace cannot be empty")
	}

	storePath := filepath.Join(workspace, "memory", "sessions")
	store, err := newJSONLStore(storePath)
	if err != nil {
		return nil, err
	}
	return NewManager(agentID, ManagerOptions{Store: store}), nil
}

func newJSONLStore(root string) (Store, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve storage path: %w", err)
	}

	if err := os.MkdirAll(absRoot, 0755); err != nil {
		return nil, fmt.Errorf("create storage path: %w", err)
	}

	return &jsonlStore{
		root:           absRoot,
		compactEvery:   defaultCompactEvery,
		compactMaxSize: defaultCompactMaxSize,
	}, nil
}

func (s *jsonlStore) Load(ctx context.Context, sessionKey string) (*Session, error) {
	_ = ctx

	lock := s.sessionLock(sessionKey)
	lock.Lock()
	defer lock.Unlock()

	path := s.sessionFile(sessionKey)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open session file: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	var (
		meta *jsonlMetadataRecord
		msgs = make([]*schema.Message, 0, 16)
	)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var header jsonlRecordHeader
		if err := sonic.UnmarshalString(line, &header); err != nil {
			return nil, fmt.Errorf("parse jsonl header: %w", err)
		}

		switch header.Type {
		case "meta":
			var m jsonlMetadataRecord
			if err := sonic.UnmarshalString(line, &m); err != nil {
				return nil, fmt.Errorf("parse metadata record: %w", err)
			}
			meta = &m
		case "msg":
			var r jsonlMessageRecord
			if err := sonic.UnmarshalString(line, &r); err != nil {
				return nil, fmt.Errorf("parse message record: %w", err)
			}
			if r.Message != nil {
				msgs = append(msgs, r.Message)
			}
		default:
			// Ignore unknown record types for forward compatibility.
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan session file: %w", err)
	}

	if meta == nil {
		meta = &jsonlMetadataRecord{
			Type:       "meta",
			SessionKey: sessionKey,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
			Format:     jsonlFormat,
			Schema:     jsonlSchema,
		}
	}
	if meta.SessionKey != "" && meta.SessionKey != sessionKey {
		return nil, fmt.Errorf("session key mismatch, want %s got %s", sessionKey, meta.SessionKey)
	}
	if !meta.ExpireAt.IsZero() && !meta.ExpireAt.After(time.Now()) {
		_ = os.Remove(path)
		return nil, nil
	}

	sess := &Session{
		SessionKey:      sessionKey,
		AgentID:         meta.AgentID,
		Channel:         channel.Type(meta.Channel),
		ChannelID:       meta.ChannelID,
		ChatID:          meta.ChatID,
		messages:        msgs,
		createTime:      meta.CreatedAt,
		updateTime:      meta.UpdatedAt,
		expireAt:        meta.ExpireAt,
		msgCnt:          meta.MsgCount,
		toolCallCnt:     meta.ToolCallCnt,
		persistedMsgLen: len(msgs),
	}
	if sess.createTime.IsZero() {
		sess.createTime = time.Now()
	}
	if sess.updateTime.IsZero() {
		sess.updateTime = sess.createTime
	}

	if sess.AgentID == "" || sess.Channel == "" || sess.ChannelID == "" || sess.ChatID == "" {
		agentID, ch, channelID, chatID, parseErr := ParseKey(sessionKey)
		if parseErr == nil {
			if sess.AgentID == "" {
				sess.AgentID = agentID
			}
			if sess.Channel == "" {
				sess.Channel = ch
			}
			if sess.ChannelID == "" {
				sess.ChannelID = channelID
			}
			if sess.ChatID == "" {
				sess.ChatID = chatID
			}
		}
	}

	return sess, nil
}

func (s *jsonlStore) Save(ctx context.Context, sess *Session) error {
	_ = ctx
	if sess == nil {
		return nil
	}

	lock := s.sessionLock(sess.SessionKey)
	lock.Lock()
	defer lock.Unlock()

	sess.mu.RLock()
	dirty := sess.dirty
	version := sess.version
	persistedMsgLen := sess.persistedMsgLen
	appendSaveCnt := sess.appendSaveCnt
	messages := make([]*schema.Message, len(sess.messages))
	copy(messages, sess.messages)

	meta := jsonlMetadataRecord{
		Type:        "meta",
		SessionKey:  sess.SessionKey,
		AgentID:     sess.AgentID,
		Channel:     string(sess.Channel),
		ChannelID:   sess.ChannelID,
		ChatID:      sess.ChatID,
		CreatedAt:   sess.createTime,
		UpdatedAt:   sess.updateTime,
		ExpireAt:    sess.expireAt,
		MsgCount:    sess.msgCnt,
		ToolCallCnt: sess.toolCallCnt,
		Format:      jsonlFormat,
		Schema:      jsonlSchema,
	}
	sess.mu.RUnlock()

	if !dirty {
		return nil
	}

	if meta.CreatedAt.IsZero() {
		meta.CreatedAt = time.Now()
	}
	if meta.UpdatedAt.IsZero() {
		meta.UpdatedAt = time.Now()
	}

	metaLine, err := sonic.MarshalString(meta)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	path := s.sessionFile(sess.SessionKey)
	currentMsgLen := len(messages)
	if persistedMsgLen > currentMsgLen {
		persistedMsgLen = currentMsgLen
	}

	needCompact := persistedMsgLen == 0
	if info, statErr := os.Stat(path); statErr != nil {
		if !os.IsNotExist(statErr) {
			return fmt.Errorf("stat session file: %w", statErr)
		}
		needCompact = true
	} else {
		if s.compactEvery > 0 && appendSaveCnt >= s.compactEvery {
			needCompact = true
		}
		if s.compactMaxSize > 0 && info.Size() >= s.compactMaxSize {
			needCompact = true
		}
	}

	if needCompact {
		if err := s.rewrite(path, metaLine, messages); err != nil {
			return err
		}
		s.markPersisted(sess, currentMsgLen, true, version)
		return nil
	}

	start := persistedMsgLen
	if start < 0 || start > currentMsgLen {
		start = 0
	}
	if err := s.appendFile(path, metaLine, messages[start:]); err != nil {
		return err
	}
	s.markPersisted(sess, currentMsgLen, false, version)
	return nil
}

func (s *jsonlStore) markPersisted(sess *Session, msgLen int, compacted bool, expectedVersion uint64) {
	sess.mu.Lock()
	defer sess.mu.Unlock()

	sess.persistedMsgLen = msgLen
	if compacted {
		sess.appendSaveCnt = 0
	} else {
		sess.appendSaveCnt++
	}
	if sess.version == expectedVersion {
		sess.dirty = false
	}
}

func (s *jsonlStore) rewrite(path string, metaLine string, messages []*schema.Message) error {
	tmpPath := path + ".tmp"
	out, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("create temp session file: %w", err)
	}
	defer func() {
		_ = out.Close()
	}()

	writer := bufio.NewWriter(out)
	if err := writeJSONLBatch(writer, metaLine, messages); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := writer.Flush(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("flush session file: %w", err)
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close session file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("replace session file: %w", err)
	}

	return nil
}

func (s *jsonlStore) appendFile(path string, metaLine string, messages []*schema.Message) error {
	out, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open session file for append: %w", err)
	}
	defer out.Close()

	writer := bufio.NewWriter(out)
	if err := writeJSONLBatch(writer, metaLine, messages); err != nil {
		return err
	}
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("flush appended session records: %w", err)
	}
	return nil
}

func writeJSONLBatch(writer *bufio.Writer, metaLine string, messages []*schema.Message) error {
	if _, err := writer.WriteString(metaLine + "\n"); err != nil {
		return fmt.Errorf("write metadata line: %w", err)
	}
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		line, err := marshalMessageLine(msg)
		if err != nil {
			return err
		}
		if _, err := writer.WriteString(line + "\n"); err != nil {
			return fmt.Errorf("write message line: %w", err)
		}
	}
	return nil
}

func marshalMessageLine(msg *schema.Message) (string, error) {
	trimMsg := &schema.Message{
		Role:      msg.Role,
		Content:   msg.Content,
		ToolCalls: msg.ToolCalls,
	}
	rec := jsonlMessageRecord{
		Type:    "msg",
		Message: trimMsg,
	}
	line, err := sonic.MarshalString(rec)
	if err != nil {
		return "", fmt.Errorf("marshal message record: %w", err)
	}
	return line, nil
}

func (s *jsonlStore) Delete(ctx context.Context, sessionKey string) error {
	_ = ctx
	lock := s.sessionLock(sessionKey)
	lock.Lock()
	defer lock.Unlock()

	path := s.sessionFile(sessionKey)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete session file: %w", err)
	}
	return nil
}

func (s *jsonlStore) sessionLock(sessionKey string) *sync.Mutex {
	if existing, ok := s.sessionLocks.Load(sessionKey); ok {
		return existing.(*sync.Mutex)
	}
	created := &sync.Mutex{}
	actual, _ := s.sessionLocks.LoadOrStore(sessionKey, created)
	return actual.(*sync.Mutex)
}

func (s *jsonlStore) GC(ctx context.Context, now time.Time) (int, error) {
	_ = ctx

	entries, err := os.ReadDir(s.root)
	if err != nil {
		return 0, fmt.Errorf("read session dir: %w", err)
	}

	removed := 0
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
			continue
		}
		path := filepath.Join(s.root, entry.Name())
		exp, ok, parseErr := readExpireAtFromJSONL(path)
		if parseErr != nil {
			continue
		}
		if !ok || exp.IsZero() || exp.After(now) {
			continue
		}
		if rmErr := os.Remove(path); rmErr == nil || os.IsNotExist(rmErr) {
			removed++
		}
	}

	return removed, nil
}

func (s *jsonlStore) sessionFile(sessionKey string) string {
	sum := sha1.Sum([]byte(sessionKey))
	filename := hex.EncodeToString(sum[:]) + ".jsonl"
	return filepath.Join(s.root, filename)
}

func readExpireAtFromJSONL(path string) (time.Time, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return time.Time{}, false, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 8*1024), 512*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var header jsonlRecordHeader
		if err := sonic.UnmarshalString(line, &header); err != nil {
			return time.Time{}, false, err
		}
		if header.Type != "meta" {
			continue
		}

		var meta jsonlMetadataRecord
		if err := sonic.UnmarshalString(line, &meta); err != nil {
			return time.Time{}, false, err
		}
		return meta.ExpireAt, true, nil
	}
	if err := scanner.Err(); err != nil {
		return time.Time{}, false, err
	}

	return time.Time{}, false, nil
}
