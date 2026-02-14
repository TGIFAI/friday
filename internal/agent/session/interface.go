package session

import (
	"context"
	"time"
)

// Store provides persistent storage for sessions.
type Store interface {
	Load(ctx context.Context, sessionKey string) (*Session, error)
	Save(ctx context.Context, sess *Session) error
	Delete(ctx context.Context, sessionKey string) error
	GC(ctx context.Context, now time.Time) (int, error)
}
