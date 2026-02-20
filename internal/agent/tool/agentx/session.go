package agentx

import (
	"fmt"
	"sync"
	"time"
)

const (
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
)

// Session represents a single agent execution context.
type Session struct {
	ID           string
	Backend      string
	CLISessionID string
	Status       string
	WorkingDir   string
	CreatedAt    time.Time
	LastOutput   string
	process      *Process // nil for sync sessions
}

// SessionManager manages a set of sessions with optional capacity limits.
type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	max      int
}

// NewSessionManager returns a SessionManager that allows up to maxSessions
// concurrent sessions. A maxSessions value of 0 means unlimited.
func NewSessionManager(maxSessions int) *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*Session),
		max:      maxSessions,
	}
}

// Create adds a new session without checking capacity limits.
// It uses the package-level seq counter to generate unique IDs like "as-1", "as-2", etc.
func (sm *SessionManager) Create(backend, workingDir string) *Session {
	id := fmt.Sprintf("as-%d", seq.Add(1))
	s := &Session{
		ID:         id,
		Backend:    backend,
		Status:     StatusRunning,
		WorkingDir: workingDir,
		CreatedAt:  time.Now(),
	}
	sm.mu.Lock()
	sm.sessions[id] = s
	sm.mu.Unlock()
	return s
}

// CreateWithLimit creates a session but returns an error if the manager
// has already reached its maximum number of sessions.
func (sm *SessionManager) CreateWithLimit(backend, workingDir string) (*Session, error) {
	sm.mu.Lock()
	if sm.max > 0 && len(sm.sessions) >= sm.max {
		sm.mu.Unlock()
		return nil, fmt.Errorf("max sessions reached (%d)", sm.max)
	}
	sm.mu.Unlock()
	return sm.Create(backend, workingDir), nil
}

// Get retrieves a session by ID. The second return value indicates whether
// the session was found.
func (sm *SessionManager) Get(id string) (*Session, bool) {
	sm.mu.RLock()
	s, ok := sm.sessions[id]
	sm.mu.RUnlock()
	return s, ok
}

// List returns all sessions. The order is non-deterministic.
func (sm *SessionManager) List() []*Session {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	list := make([]*Session, 0, len(sm.sessions))
	for _, s := range sm.sessions {
		list = append(list, s)
	}
	return list
}

// Destroy removes a session by ID. If the session has a running process,
// it is killed first.
func (sm *SessionManager) Destroy(id string) {
	sm.mu.Lock()
	s, ok := sm.sessions[id]
	if ok {
		delete(sm.sessions, id)
	}
	sm.mu.Unlock()

	if ok && s.process != nil {
		s.process.Kill()
	}
}
