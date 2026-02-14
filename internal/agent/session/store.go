package session

import (
	"context"
	"fmt"
	"path/filepath"
	"time"
)

// Store provides persistent storage for sessions.
type Store interface {
	Load(ctx context.Context, sessionKey string) (*Session, error)
	Save(ctx context.Context, sess *Session) error
	Delete(ctx context.Context, sessionKey string) error
	GC(ctx context.Context, now time.Time) (int, error)
}

func NewAgentJSONLStore(workspace string) (Store, error) {
	if workspace == "" {
		return nil, fmt.Errorf("workspace cannot be empty")
	}

	storePath := filepath.Join(workspace, "memory", "sessions")
	store, err := NewJSONLStore(storePath)
	if err != nil {
		return nil, err
	}

	return store, nil
}

func NewJSONLManager(agentID string, workspace string) (*Manager, error) {
	store, err := NewAgentJSONLStore(workspace)
	if err != nil {
		return nil, err
	}
	return NewManager(agentID, ManagerOptions{Store: store}), nil
}
