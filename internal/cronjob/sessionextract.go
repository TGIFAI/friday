package cronjob

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bytedance/sonic"
)

// SessionExcerpt holds user messages extracted from a single session file.
type SessionExcerpt struct {
	SessionKey string
	Messages   []string
}

// sessionMetaRecord mirrors the metadata fields we need from JSONL session files.
type sessionMetaRecord struct {
	Type       string    `json:"_type"`
	SessionKey string    `json:"session_key"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// sessionMsgRecord mirrors message records in JSONL session files.
type sessionMsgRecord struct {
	Type    string          `json:"_type"`
	Message *sessionMsgBody `json:"msg"`
}

type sessionMsgBody struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ExtractUserMessages scans JSONL session files under sessionsDir and returns
// user messages from sessions that were active on targetDate. It skips cron
// sessions (session key containing "cron:") and limits output per session.
func ExtractUserMessages(sessionsDir string, targetDate time.Time, maxPerSession int) ([]SessionExcerpt, error) {
	dayStart := time.Date(targetDate.Year(), targetDate.Month(), targetDate.Day(), 0, 0, 0, 0, targetDate.Location())
	dayEnd := dayStart.AddDate(0, 0, 1)

	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var excerpts []SessionExcerpt
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
			continue
		}

		// Quick mtime pre-filter: skip files not modified since dayStart.
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(dayStart) {
			continue
		}

		path := filepath.Join(sessionsDir, entry.Name())
		excerpt, err := extractFromFile(path, dayStart, dayEnd, maxPerSession)
		if err != nil || excerpt == nil {
			continue
		}
		excerpts = append(excerpts, *excerpt)
	}

	return excerpts, nil
}

func extractFromFile(path string, dayStart, dayEnd time.Time, maxPerSession int) (*SessionExcerpt, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	// A JSONL session file may contain multiple meta records (each append-save
	// rewrites the meta line). We collect all user messages and use the *last*
	// meta record for session key / date filtering — that record reflects the
	// most recent state.
	var (
		lastMeta *sessionMetaRecord
		messages []string
	)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// Peek at the _type field.
		var header struct {
			Type string `json:"_type"`
		}
		if err := sonic.UnmarshalString(line, &header); err != nil {
			continue
		}

		switch header.Type {
		case "meta":
			var meta sessionMetaRecord
			if err := sonic.UnmarshalString(line, &meta); err != nil {
				return nil, err
			}
			lastMeta = &meta

		case "msg":
			var rec sessionMsgRecord
			if err := sonic.UnmarshalString(line, &rec); err != nil {
				continue
			}
			if rec.Message == nil || rec.Message.Role != "user" {
				continue
			}
			content := strings.TrimSpace(rec.Message.Content)
			if content == "" {
				continue
			}
			if maxPerSession > 0 && len(messages) >= maxPerSession {
				continue
			}
			messages = append(messages, content)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// Apply filters using the last (most recent) meta record.
	if lastMeta == nil || len(messages) == 0 {
		return nil, nil
	}
	// Skip cron sessions.
	if strings.Contains(lastMeta.SessionKey, "cron:") {
		return nil, nil
	}
	// Check if session was active on the target date.
	if lastMeta.UpdatedAt.Before(dayStart) || !lastMeta.UpdatedAt.Before(dayEnd) {
		return nil, nil
	}

	return &SessionExcerpt{
		SessionKey: lastMeta.SessionKey,
		Messages:   messages,
	}, nil
}
