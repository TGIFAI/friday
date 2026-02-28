package browserx

import (
	"fmt"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/google/uuid"

	"github.com/tgifai/friday/internal/pkg/logs"
)

// Session represents a browser instance with its active page.
type Session struct {
	ID        string
	Browser   *rod.Browser
	Page      *rod.Page
	Headless  bool
	CreatedAt time.Time
	mu        sync.Mutex
}

// BrowserManager manages multiple browser sessions.
type BrowserManager struct {
	sessions map[string]*Session
	mu       sync.RWMutex
}

var (
	globalManager     *BrowserManager
	globalManagerOnce sync.Once
)

func getGlobalManager() *BrowserManager {
	globalManagerOnce.Do(func() {
		globalManager = newBrowserManager()
	})
	return globalManager
}

func newBrowserManager() *BrowserManager {
	return &BrowserManager{
		sessions: make(map[string]*Session),
	}
}

func (m *BrowserManager) openSession(headless bool) (*Session, error) {
	if !headless {
		if ok, reason := canRunHeaded(); !ok {
			return nil, fmt.Errorf("headed mode not supported: %s. Use headless=true instead", reason)
		}
	}

	l := newStealthLauncher(headless)
	controlURL, err := l.Launch()
	if err != nil {
		return nil, fmt.Errorf("launch browser: %w", err)
	}

	browser := rod.New().ControlURL(controlURL)
	if err := browser.Connect(); err != nil {
		return nil, fmt.Errorf("connect to browser: %w", err)
	}

	configureBrowser(browser)

	page, err := newStealthPage(browser)
	if err != nil {
		browser.Close()
		return nil, fmt.Errorf("create stealth page: %w", err)
	}

	if err := setViewport(page); err != nil {
		browser.Close()
		return nil, fmt.Errorf("set viewport: %w", err)
	}

	id := uuid.New().String()[:8]

	sess := &Session{
		ID:        id,
		Browser:   browser,
		Page:      page,
		Headless:  headless,
		CreatedAt: time.Now(),
	}

	m.mu.Lock()
	m.sessions[id] = sess
	m.mu.Unlock()

	logs.Info("[tool:browser] opened session %s (headless=%v)", id, headless)
	return sess, nil
}

func (m *BrowserManager) getSession(id string) (*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sess, ok := m.sessions[id]
	if !ok {
		return nil, fmt.Errorf("session '%s' not found. Use operation=open to create one, or operation=list_sessions to see active sessions", id)
	}
	return sess, nil
}

func (m *BrowserManager) closeSession(id string) error {
	m.mu.Lock()
	sess, ok := m.sessions[id]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("session '%s' not found", id)
	}
	delete(m.sessions, id)
	m.mu.Unlock()

	sess.mu.Lock()
	defer sess.mu.Unlock()

	if err := sess.Browser.Close(); err != nil {
		logs.Warn("[tool:browser] error closing session %s: %v", id, err)
		return err
	}

	logs.Info("[tool:browser] closed session %s", id)
	return nil
}

func (m *BrowserManager) listSessions() []map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]map[string]interface{}, 0, len(m.sessions))
	for _, sess := range m.sessions {
		entry := map[string]interface{}{
			"id":         sess.ID,
			"headless":   sess.Headless,
			"created_at": sess.CreatedAt.Format(time.RFC3339),
		}

		sess.mu.Lock()
		if sess.Page != nil {
			info, err := sess.Page.Info()
			if err == nil && info != nil {
				entry["current_url"] = info.URL
				entry["title"] = info.Title
			}
		}
		sess.mu.Unlock()

		result = append(result, entry)
	}
	return result
}

func (m *BrowserManager) shutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, sess := range m.sessions {
		sess.mu.Lock()
		if err := sess.Browser.Close(); err != nil {
			logs.Warn("[tool:browser] shutdown error for session %s: %v", id, err)
		}
		sess.mu.Unlock()
	}
	m.sessions = make(map[string]*Session)
	logs.Info("[tool:browser] all sessions closed")
}
