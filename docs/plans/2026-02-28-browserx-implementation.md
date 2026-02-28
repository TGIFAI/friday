# browserx Tool Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement a browser automation tool (`browserx`) for the friday agent framework using go-rod with layered anti-detection.

**Architecture:** Single tool with `operation` parameter dispatch (like cronx). A global `BrowserManager` singleton manages browser sessions that persist across agent loops. Stealth is layered: Chrome launch flags → go-rod/stealth JS → incremental JS supplements → behavioral defaults.

**Tech Stack:** go-rod/rod, go-rod/stealth, Go 1.24, cloudwego/eino schema, bytedance/gg for type conversion, bytedance/sonic for JSON.

---

### Task 1: Add Dependencies

**Files:**
- Modify: `go.mod`

**Step 1: Add go-rod and stealth dependencies**

Run:
```bash
cd /Users/dave/go/src/github.com/tgifai/friday && go get github.com/go-rod/rod@latest github.com/go-rod/stealth@latest
```

**Step 2: Tidy**

Run:
```bash
go mod tidy
```

**Step 3: Verify import works**

Run:
```bash
go build ./...
```
Expected: compiles with no errors

**Step 4: Commit**

```bash
git add go.mod go.sum
git commit -m "feat(browserx): add go-rod and stealth dependencies"
```

---

### Task 2: Environment Detection (env.go)

**Files:**
- Create: `internal/agent/tool/browserx/env.go`
- Create: `internal/agent/tool/browserx/env_test.go`

**Step 1: Write the test**

```go
package browserx

import (
	"os"
	"runtime"
	"testing"
)

func TestCanRunHeaded(t *testing.T) {
	ok, reason := canRunHeaded()

	switch runtime.GOOS {
	case "darwin":
		// On macOS with a local terminal, should be true.
		if os.Getenv("SSH_CONNECTION") == "" {
			if !ok {
				t.Errorf("expected headed to be supported on macOS local, got reason: %s", reason)
			}
		}
	case "linux":
		display := os.Getenv("DISPLAY")
		wayland := os.Getenv("WAYLAND_DISPLAY")
		if display == "" && wayland == "" {
			if ok {
				t.Error("expected headed NOT supported on linux without DISPLAY/WAYLAND_DISPLAY")
			}
		} else {
			if !ok {
				t.Errorf("expected headed supported on linux with display, got reason: %s", reason)
			}
		}
	case "windows":
		if !ok {
			t.Errorf("expected headed supported on windows, got reason: %s", reason)
		}
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run TestCanRunHeaded ./internal/agent/tool/browserx/...`
Expected: FAIL (package doesn't exist yet)

**Step 3: Write env.go**

```go
package browserx

import (
	"fmt"
	"os"
	"runtime"
)

// canRunHeaded checks if the current environment supports headed (GUI) browser mode.
// Returns (true, "") if supported, or (false, reason) if not.
func canRunHeaded() (bool, string) {
	switch runtime.GOOS {
	case "darwin":
		if os.Getenv("SSH_CONNECTION") != "" && os.Getenv("DISPLAY") == "" {
			return false, "macOS SSH session without display forwarding"
		}
		return true, ""

	case "linux":
		display := os.Getenv("DISPLAY")
		waylandDisplay := os.Getenv("WAYLAND_DISPLAY")
		if display == "" && waylandDisplay == "" {
			return false, "no DISPLAY or WAYLAND_DISPLAY set (headless Linux server?)"
		}
		return true, ""

	case "windows":
		return true, ""

	default:
		return false, fmt.Sprintf("unsupported OS: %s", runtime.GOOS)
	}
}
```

**Step 4: Run test to verify it passes**

Run: `go test -v -run TestCanRunHeaded ./internal/agent/tool/browserx/...`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/agent/tool/browserx/env.go internal/agent/tool/browserx/env_test.go
git commit -m "feat(browserx): add environment detection for headed mode"
```

---

### Task 3: Stealth Layer (stealth.go)

**Files:**
- Create: `internal/agent/tool/browserx/stealth.go`

**Step 1: Write stealth.go**

This file provides two functions:
1. `newStealthLauncher(headless bool)` — configures Chrome launch flags for anti-detection
2. `applyExtraStealth(page *rod.Page)` — injects additional JS evasions on top of go-rod/stealth

```go
package browserx

import (
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/stealth"
)

// newStealthLauncher creates a launcher with anti-detection Chrome flags.
func newStealthLauncher(headless bool) *launcher.Launcher {
	l := launcher.New()

	if headless {
		l.Headless(true)
		// Use new headless mode (Chrome 112+) which is harder to detect.
		l.Set("headless", "new")
	} else {
		l.Headless(false)
	}

	// Remove automation markers.
	l.Set("disable-blink-features", "AutomationControlled")

	// Disable features that leak automation.
	l.Set("disable-features", "TranslateUI")
	l.Set("disable-infobars")
	l.Set("disable-dev-shm-usage")
	l.Set("no-first-run")
	l.Set("no-default-browser-check")

	// Realistic window size (headless defaults to 800x600 which is a giveaway).
	l.Set("window-size", "1920,1080")

	// Language.
	l.Set("lang", "en-US")

	return l
}

// newStealthPage creates a page using go-rod/stealth and applies extra evasions.
func newStealthPage(browser *rod.Browser) (*rod.Page, error) {
	page, err := stealth.Page(browser)
	if err != nil {
		return nil, err
	}

	if err := applyExtraStealth(page); err != nil {
		page.Close()
		return nil, err
	}

	return page, nil
}

// applyExtraStealth injects additional JS evasions beyond what go-rod/stealth covers.
func applyExtraStealth(page *rod.Page) error {
	// Layer 3: Incremental JS supplements.
	js := `
	// Fix window.outerWidth/outerHeight (may be 0 in headless).
	if (window.outerWidth === 0) {
		Object.defineProperty(window, 'outerWidth', { get: () => window.innerWidth });
	}
	if (window.outerHeight === 0) {
		Object.defineProperty(window, 'outerHeight', { get: () => window.innerHeight + 85 });
	}

	// Simulate navigator.connection (NetworkInformation API).
	if (!navigator.connection) {
		Object.defineProperty(navigator, 'connection', {
			get: () => ({
				effectiveType: '4g',
				rtt: 50,
				downlink: 10,
				saveData: false,
			}),
		});
	}

	// Ensure navigator.permissions.query returns consistent results for notifications.
	const originalQuery = window.navigator.permissions.query.bind(window.navigator.permissions);
	window.navigator.permissions.query = (parameters) => {
		if (parameters.name === 'notifications') {
			return Promise.resolve({ state: Notification.permission });
		}
		return originalQuery(parameters);
	};
	`

	_, err := page.EvalOnNewDocument(js)
	return err
}

// configureBrowser sets realistic device defaults on the browser.
func configureBrowser(browser *rod.Browser) {
	// Disable default device emulation to avoid touch simulation artifacts.
	// Instead we'll set viewport via CDP when creating pages.
	browser.DefaultDevice(rod.DeviceType{})
}

// setViewport sets a realistic viewport on the page.
func setViewport(page *rod.Page) error {
	return page.SetViewport(&proto.EmulationSetDeviceMetricsOverride{
		Width:             1920,
		Height:            1080,
		DeviceScaleFactor: 1,
		Mobile:            false,
	})
}
```

**Step 2: Verify compilation**

Run: `go build ./internal/agent/tool/browserx/...`
Expected: compiles

**Step 3: Commit**

```bash
git add internal/agent/tool/browserx/stealth.go
git commit -m "feat(browserx): add stealth layer with launcher flags and JS evasions"
```

---

### Task 4: Session Manager (manager.go)

**Files:**
- Create: `internal/agent/tool/browserx/manager.go`
- Create: `internal/agent/tool/browserx/manager_test.go`

**Step 1: Write the test**

```go
package browserx

import (
	"testing"
)

func TestManagerGetSession(t *testing.T) {
	mgr := newBrowserManager()

	// Getting a non-existent session should fail.
	_, err := mgr.getSession("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent session")
	}

	// List should be empty.
	sessions := mgr.listSessions()
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestManagerShutdownEmpty(t *testing.T) {
	mgr := newBrowserManager()
	// Shutdown with no sessions should not panic.
	mgr.shutdown()
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run TestManager ./internal/agent/tool/browserx/...`
Expected: FAIL (manager.go doesn't exist)

**Step 3: Write manager.go**

```go
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

// global singleton
var (
	globalManager     *BrowserManager
	globalManagerOnce sync.Once
)

// getGlobalManager returns the singleton BrowserManager.
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

// openSession creates a new browser session.
func (m *BrowserManager) openSession(headless bool) (*Session, error) {
	// Environment check for headed mode.
	if !headless {
		if ok, reason := canRunHeaded(); !ok {
			return nil, fmt.Errorf("headed mode not supported: %s. Use headless=true instead", reason)
		}
	}

	// Launch browser with stealth flags.
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

	// Create stealth page.
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

// getSession retrieves a session by ID.
func (m *BrowserManager) getSession(id string) (*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sess, ok := m.sessions[id]
	if !ok {
		return nil, fmt.Errorf("session '%s' not found. Use operation=open to create one, or operation=list_sessions to see active sessions", id)
	}
	return sess, nil
}

// closeSession closes a browser session and removes it from the manager.
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

// listSessions returns info about all active sessions.
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

// shutdown closes all browser sessions. Called on program exit.
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
```

**Step 4: Run tests**

Run: `go test -v -run TestManager ./internal/agent/tool/browserx/...`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/agent/tool/browserx/manager.go internal/agent/tool/browserx/manager_test.go
git commit -m "feat(browserx): add session manager with lifecycle management"
```

---

### Task 5: Operations (ops.go)

**Files:**
- Create: `internal/agent/tool/browserx/ops.go`

**Step 1: Write ops.go**

This file implements all 11 operations. Each operation is a method on `BrowserTool` that takes `context.Context` and `map[string]interface{}` args.

Reference patterns:
- Use `gconv.To[string](args["key"])` for string extraction (like cronx/cron.go:87)
- Use `gconv.To[float64](args["timeout"])` for numeric values (like shellx/exec.go:159)
- Return `map[string]interface{}` results (like cronx/cron.go:162)

```go
package browserx

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/bytedance/gg/gconv"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"

	"github.com/tgifai/friday/internal/pkg/logs"
)

const (
	defaultTimeout = 30 * time.Second
)

func (t *BrowserTool) opOpen(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	headless := true
	if v, ok := args["headless"]; ok {
		headless = gconv.To[bool](v)
	}

	sess, err := t.manager.openSession(headless)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"session_id": sess.ID,
		"headless":   sess.Headless,
		"stealth":    true,
	}, nil
}

func (t *BrowserTool) opClose(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	sessionID := gconv.To[string](args["session_id"])
	if sessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}

	if err := t.manager.closeSession(sessionID); err != nil {
		return nil, err
	}

	return map[string]interface{}{"success": true}, nil
}

func (t *BrowserTool) opNavigate(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	sess, err := t.requireSession(args)
	if err != nil {
		return nil, err
	}

	url := gconv.To[string](args["url"])
	if url == "" {
		return nil, fmt.Errorf("url is required for navigate")
	}

	waitLoad := true
	if v, ok := args["wait_load"]; ok {
		waitLoad = gconv.To[bool](v)
	}

	sess.mu.Lock()
	defer sess.mu.Unlock()

	if err := sess.Page.Navigate(url); err != nil {
		return nil, fmt.Errorf("navigate to %s: %w", url, err)
	}

	if waitLoad {
		if err := sess.Page.WaitLoad(); err != nil {
			logs.CtxWarn(ctx, "[tool:browser] WaitLoad warning: %v", err)
		}
	}

	info, err := sess.Page.Info()
	if err != nil {
		return nil, fmt.Errorf("get page info: %w", err)
	}

	return map[string]interface{}{
		"url":   info.URL,
		"title": info.Title,
	}, nil
}

func (t *BrowserTool) opClick(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	sess, err := t.requireSession(args)
	if err != nil {
		return nil, err
	}

	selector := gconv.To[string](args["selector"])
	if selector == "" {
		return nil, fmt.Errorf("selector is required for click")
	}

	sess.mu.Lock()
	defer sess.mu.Unlock()

	el, err := t.findElement(sess.Page, selector, args)
	if err != nil {
		return nil, err
	}

	if err := el.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return nil, fmt.Errorf("click %s: %w", selector, err)
	}

	return map[string]interface{}{
		"success":  true,
		"selector": selector,
	}, nil
}

func (t *BrowserTool) opType(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	sess, err := t.requireSession(args)
	if err != nil {
		return nil, err
	}

	selector := gconv.To[string](args["selector"])
	if selector == "" {
		return nil, fmt.Errorf("selector is required for type")
	}

	text := gconv.To[string](args["text"])
	if text == "" {
		return nil, fmt.Errorf("text is required for type")
	}

	clearFirst := gconv.To[bool](args["clear"])

	sess.mu.Lock()
	defer sess.mu.Unlock()

	el, err := t.findElement(sess.Page, selector, args)
	if err != nil {
		return nil, err
	}

	if clearFirst {
		if err := el.SelectAllText(); err != nil {
			return nil, fmt.Errorf("select all text: %w", err)
		}
	}

	if err := el.Input(text); err != nil {
		return nil, fmt.Errorf("type into %s: %w", selector, err)
	}

	return map[string]interface{}{"success": true}, nil
}

func (t *BrowserTool) opScreenshot(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	sess, err := t.requireSession(args)
	if err != nil {
		return nil, err
	}

	format := proto.PageCaptureScreenshotFormatPng
	if f := gconv.To[string](args["format"]); strings.ToLower(f) == "jpeg" || strings.ToLower(f) == "jpg" {
		format = proto.PageCaptureScreenshotFormatJpeg
	}

	sess.mu.Lock()
	defer sess.mu.Unlock()

	var data []byte

	selector := gconv.To[string](args["selector"])
	if selector != "" {
		el, findErr := t.findElement(sess.Page, selector, args)
		if findErr != nil {
			return nil, findErr
		}
		data, err = el.Screenshot(format, 90)
	} else {
		data, err = sess.Page.Screenshot(false, &proto.PageCaptureScreenshot{
			Format: format,
		})
	}

	if err != nil {
		return nil, fmt.Errorf("screenshot: %w", err)
	}

	return map[string]interface{}{
		"data":   base64.StdEncoding.EncodeToString(data),
		"format": string(format),
	}, nil
}

func (t *BrowserTool) opExtract(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	sess, err := t.requireSession(args)
	if err != nil {
		return nil, err
	}

	selector := gconv.To[string](args["selector"])
	if selector == "" {
		return nil, fmt.Errorf("selector is required for extract")
	}

	attribute := gconv.To[string](args["attribute"])
	all := gconv.To[bool](args["all"])

	sess.mu.Lock()
	defer sess.mu.Unlock()

	if all {
		elements, findErr := sess.Page.Elements(selector)
		if findErr != nil {
			return nil, fmt.Errorf("find elements %s: %w", selector, findErr)
		}

		var results []map[string]interface{}
		for _, el := range elements {
			entry := map[string]interface{}{}
			if attribute != "" {
				val, _ := el.Attribute(attribute)
				if val != nil {
					entry["attribute"] = *val
				}
			}
			text, _ := el.Text()
			entry["text"] = text
			html, _ := el.HTML()
			entry["html"] = html
			results = append(results, entry)
		}

		return map[string]interface{}{
			"elements": results,
			"count":    len(results),
		}, nil
	}

	el, findErr := t.findElement(sess.Page, selector, args)
	if findErr != nil {
		return nil, findErr
	}

	result := map[string]interface{}{}
	if attribute != "" {
		val, _ := el.Attribute(attribute)
		if val != nil {
			result["attribute"] = *val
		}
	}
	text, _ := el.Text()
	result["text"] = text
	html, _ := el.HTML()
	result["html"] = html

	return result, nil
}

func (t *BrowserTool) opEvaluate(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	sess, err := t.requireSession(args)
	if err != nil {
		return nil, err
	}

	script := gconv.To[string](args["script"])
	if script == "" {
		return nil, fmt.Errorf("script is required for evaluate")
	}

	sess.mu.Lock()
	defer sess.mu.Unlock()

	// Wrap in an arrow function if not already a function expression.
	if !strings.HasPrefix(strings.TrimSpace(script), "(") && !strings.HasPrefix(strings.TrimSpace(script), "function") {
		script = fmt.Sprintf("() => { %s }", script)
	}

	result, err := sess.Page.Eval(script)
	if err != nil {
		return nil, fmt.Errorf("evaluate: %w", err)
	}

	return map[string]interface{}{
		"result": result.Value.Val(),
	}, nil
}

func (t *BrowserTool) opWait(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	sess, err := t.requireSession(args)
	if err != nil {
		return nil, err
	}

	selector := gconv.To[string](args["selector"])
	if selector == "" {
		return nil, fmt.Errorf("selector is required for wait")
	}

	timeout := defaultTimeout
	if v := gconv.To[float64](args["timeout"]); v > 0 {
		timeout = time.Duration(v * float64(time.Second))
	}

	sess.mu.Lock()
	defer sess.mu.Unlock()

	page := sess.Page.Timeout(timeout)

	selectorType := gconv.To[string](args["selector_type"])
	var el *rod.Element
	if selectorType == "xpath" {
		el, err = page.ElementX(selector)
	} else {
		el, err = page.Element(selector)
	}

	if err != nil {
		return nil, fmt.Errorf("element not found: selector '%s' timed out after %v", selector, timeout)
	}

	if err := el.WaitVisible(); err != nil {
		return nil, fmt.Errorf("element '%s' not visible after %v", selector, timeout)
	}

	return map[string]interface{}{"success": true}, nil
}

func (t *BrowserTool) opScroll(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	sess, err := t.requireSession(args)
	if err != nil {
		return nil, err
	}

	sess.mu.Lock()
	defer sess.mu.Unlock()

	selector := gconv.To[string](args["selector"])
	if selector != "" {
		el, findErr := t.findElement(sess.Page, selector, args)
		if findErr != nil {
			return nil, findErr
		}
		if err := el.ScrollIntoView(); err != nil {
			return nil, fmt.Errorf("scroll to element: %w", err)
		}
		return map[string]interface{}{"success": true}, nil
	}

	x := gconv.To[float64](args["x"])
	y := gconv.To[float64](args["y"])

	if err := sess.Page.Mouse.Scroll(x, y, 0); err != nil {
		return nil, fmt.Errorf("scroll: %w", err)
	}

	return map[string]interface{}{"success": true}, nil
}

func (t *BrowserTool) opListSessions(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	sessions := t.manager.listSessions()
	return map[string]interface{}{
		"sessions": sessions,
		"count":    len(sessions),
	}, nil
}

// --- helpers ---

// requireSession extracts session_id from args and retrieves the session.
func (t *BrowserTool) requireSession(args map[string]interface{}) (*Session, error) {
	sessionID := gconv.To[string](args["session_id"])
	if sessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	return t.manager.getSession(sessionID)
}

// findElement locates an element using CSS or XPath selector.
func (t *BrowserTool) findElement(page *rod.Page, selector string, args map[string]interface{}) (*rod.Element, error) {
	selectorType := gconv.To[string](args["selector_type"])
	var el *rod.Element
	var err error

	timedPage := page.Timeout(defaultTimeout)

	if selectorType == "xpath" {
		el, err = timedPage.ElementX(selector)
	} else {
		el, err = timedPage.Element(selector)
	}

	if err != nil {
		return nil, fmt.Errorf("element not found: selector '%s' timed out after %v", selector, defaultTimeout)
	}
	return el, nil
}
```

**Step 2: Verify compilation**

Run: `go build ./internal/agent/tool/browserx/...`
Expected: compiles

**Step 3: Commit**

```bash
git add internal/agent/tool/browserx/ops.go
git commit -m "feat(browserx): add all 11 operations"
```

---

### Task 6: Tool Entry Point (browser.go)

**Files:**
- Create: `internal/agent/tool/browserx/browser.go`

**Step 1: Write browser.go**

This is the main tool entry point implementing the `tool.Tool` interface.

```go
package browserx

import (
	"context"
	"fmt"
	"strings"

	"github.com/bytedance/gg/gconv"
	"github.com/cloudwego/eino/schema"
)

// BrowserTool implements the tool.Tool interface for browser automation.
type BrowserTool struct {
	manager *BrowserManager
}

// NewBrowserTool creates a new BrowserTool backed by the global BrowserManager.
func NewBrowserTool() *BrowserTool {
	return &BrowserTool{
		manager: getGlobalManager(),
	}
}

func (t *BrowserTool) Name() string { return "browser" }

func (t *BrowserTool) Description() string {
	return "Browser automation tool with stealth anti-detection. Supports navigation, element interaction, screenshots, content extraction, and JavaScript evaluation."
}

func (t *BrowserTool) ToolInfo() *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: t.Name(),
		Desc: t.Description(),
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"operation": {
				Type:     schema.String,
				Desc:     `Operation to perform: "open", "close", "navigate", "click", "type", "screenshot", "extract", "evaluate", "wait", "scroll", "list_sessions"`,
				Required: true,
			},
			"session_id": {
				Type: schema.String,
				Desc: "Browser session ID (returned by open, required for all operations except open and list_sessions)",
			},
			"url": {
				Type: schema.String,
				Desc: "URL to navigate to (for navigate operation)",
			},
			"selector": {
				Type: schema.String,
				Desc: "CSS or XPath selector (for click, type, extract, wait, scroll, screenshot)",
			},
			"selector_type": {
				Type: schema.String,
				Desc: `Selector type: "css" (default) or "xpath"`,
			},
			"text": {
				Type: schema.String,
				Desc: "Text to input (for type operation)",
			},
			"script": {
				Type: schema.String,
				Desc: "JavaScript code to execute (for evaluate operation)",
			},
			"headless": {
				Type: schema.Boolean,
				Desc: "Run browser in headless mode (default: true, for open operation)",
			},
			"format": {
				Type: schema.String,
				Desc: `Screenshot format: "png" (default) or "jpeg" (for screenshot operation)`,
			},
			"clear": {
				Type: schema.Boolean,
				Desc: "Clear existing text before typing (for type operation)",
			},
			"attribute": {
				Type: schema.String,
				Desc: "HTML attribute to extract (for extract operation, e.g. href, src)",
			},
			"all": {
				Type: schema.Boolean,
				Desc: "Extract all matching elements instead of first (for extract operation)",
			},
			"timeout": {
				Type: schema.Integer,
				Desc: "Timeout in seconds (for wait operation, default: 30)",
			},
			"x": {
				Type: schema.Number,
				Desc: "Horizontal scroll offset in pixels (for scroll operation)",
			},
			"y": {
				Type: schema.Number,
				Desc: "Vertical scroll offset in pixels (for scroll operation)",
			},
			"wait_load": {
				Type: schema.Boolean,
				Desc: "Wait for page load after navigation (default: true, for navigate operation)",
			},
		}),
	}
}

func (t *BrowserTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	operation := strings.ToLower(strings.TrimSpace(gconv.To[string](args["operation"])))

	switch operation {
	case "open":
		return t.opOpen(ctx, args)
	case "close":
		return t.opClose(ctx, args)
	case "navigate":
		return t.opNavigate(ctx, args)
	case "click":
		return t.opClick(ctx, args)
	case "type":
		return t.opType(ctx, args)
	case "screenshot":
		return t.opScreenshot(ctx, args)
	case "extract":
		return t.opExtract(ctx, args)
	case "evaluate":
		return t.opEvaluate(ctx, args)
	case "wait":
		return t.opWait(ctx, args)
	case "scroll":
		return t.opScroll(ctx, args)
	case "list_sessions":
		return t.opListSessions(ctx, args)
	default:
		return nil, fmt.Errorf("unknown operation %q, must be one of: open, close, navigate, click, type, screenshot, extract, evaluate, wait, scroll, list_sessions", operation)
	}
}

// Shutdown closes all browser sessions. Should be called on program exit.
func Shutdown() {
	getGlobalManager().shutdown()
}
```

**Step 2: Verify compilation**

Run: `go build ./internal/agent/tool/browserx/...`
Expected: compiles

**Step 3: Commit**

```bash
git add internal/agent/tool/browserx/browser.go
git commit -m "feat(browserx): add tool entry point with operation dispatch"
```

---

### Task 7: Wire into Agent and Gateway Shutdown

**Files:**
- Modify: `internal/agent/agent.go:108-161` (add browserx import and registration)
- Modify: `internal/gateway/gateway.go:133-152` (add browserx shutdown)

**Step 1: Add browserx import and registration in agent.go**

In `internal/agent/agent.go`, add import:
```go
"github.com/tgifai/friday/internal/agent/tool/browserx"
```

In `Init()` method, add after the agentx registration (line ~159):
```go
// browser automation tools
_ = ag.tools.Register(browserx.NewBrowserTool())
```

**Step 2: Add shutdown in gateway.go**

In `internal/gateway/gateway.go`, add import:
```go
"github.com/tgifai/friday/internal/agent/tool/browserx"
```

In the `Stop()` method, add before the http server shutdown (before line 145):
```go
// Close all browser sessions.
browserx.Shutdown()
```

**Step 3: Verify build**

Run: `go build ./...`
Expected: compiles with no errors

**Step 4: Commit**

```bash
git add internal/agent/agent.go internal/gateway/gateway.go
git commit -m "feat(browserx): wire tool into agent and shutdown into gateway"
```

---

### Task 8: Integration Test

**Files:**
- Create: `internal/agent/tool/browserx/browser_test.go`

**Step 1: Write integration test**

This test verifies the full tool flow: open → navigate → extract → screenshot → evaluate → close. It requires a Chrome binary (skipped in CI if not available).

```go
package browserx

import (
	"context"
	"testing"

	"github.com/go-rod/rod/lib/launcher"
)

func TestBrowserToolIntegration(t *testing.T) {
	// Skip if no browser binary available.
	_, err := launcher.LookPath()
	if err != nil {
		t.Skip("no browser binary found, skipping integration test")
	}

	tool := &BrowserTool{manager: newBrowserManager()}
	defer tool.manager.shutdown()
	ctx := context.Background()

	// 1. Open
	result, err := tool.Execute(ctx, map[string]interface{}{
		"operation": "open",
		"headless":  true,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	openResult := result.(map[string]interface{})
	sessionID := openResult["session_id"].(string)
	if sessionID == "" {
		t.Fatal("expected non-empty session_id")
	}
	if !openResult["stealth"].(bool) {
		t.Error("expected stealth=true")
	}

	// 2. List sessions
	result, err = tool.Execute(ctx, map[string]interface{}{
		"operation": "list_sessions",
	})
	if err != nil {
		t.Fatalf("list_sessions: %v", err)
	}
	listResult := result.(map[string]interface{})
	if listResult["count"].(int) != 1 {
		t.Errorf("expected 1 session, got %v", listResult["count"])
	}

	// 3. Navigate
	result, err = tool.Execute(ctx, map[string]interface{}{
		"operation":  "navigate",
		"session_id": sessionID,
		"url":        "https://example.com",
	})
	if err != nil {
		t.Fatalf("navigate: %v", err)
	}
	navResult := result.(map[string]interface{})
	if navResult["title"] == "" {
		t.Error("expected non-empty title after navigation")
	}

	// 4. Extract
	result, err = tool.Execute(ctx, map[string]interface{}{
		"operation":  "extract",
		"session_id": sessionID,
		"selector":   "h1",
	})
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	extractResult := result.(map[string]interface{})
	if extractResult["text"] == "" {
		t.Error("expected non-empty text from h1")
	}

	// 5. Screenshot
	result, err = tool.Execute(ctx, map[string]interface{}{
		"operation":  "screenshot",
		"session_id": sessionID,
	})
	if err != nil {
		t.Fatalf("screenshot: %v", err)
	}
	ssResult := result.(map[string]interface{})
	if ssResult["data"] == "" {
		t.Error("expected non-empty screenshot data")
	}

	// 6. Evaluate
	result, err = tool.Execute(ctx, map[string]interface{}{
		"operation":  "evaluate",
		"session_id": sessionID,
		"script":     "() => document.title",
	})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	evalResult := result.(map[string]interface{})
	if evalResult["result"] == nil {
		t.Error("expected non-nil evaluate result")
	}

	// 7. Close
	result, err = tool.Execute(ctx, map[string]interface{}{
		"operation":  "close",
		"session_id": sessionID,
	})
	if err != nil {
		t.Fatalf("close: %v", err)
	}

	// 8. Verify session is gone
	_, err = tool.Execute(ctx, map[string]interface{}{
		"operation":  "navigate",
		"session_id": sessionID,
		"url":        "https://example.com",
	})
	if err == nil {
		t.Error("expected error when using closed session")
	}
}
```

**Step 2: Run integration test**

Run: `go test -v -run TestBrowserToolIntegration ./internal/agent/tool/browserx/... -timeout 60s`
Expected: PASS (or SKIP if no browser binary)

**Step 3: Commit**

```bash
git add internal/agent/tool/browserx/browser_test.go
git commit -m "test(browserx): add integration test for full tool flow"
```

---

### Task 9: Build Verification and Final Check

**Step 1: Full build**

Run: `go build -trimpath -ldflags="-s -w" ./cmd/friday`
Expected: compiles

**Step 2: Run all tests**

Run: `go test ./internal/agent/tool/browserx/... -v -timeout 120s`
Expected: all PASS

**Step 3: Run full project tests to ensure no regressions**

Run: `go test ./... -timeout 300s`
Expected: no new failures

**Step 4: Final commit (if any formatting/lint fixes needed)**

```bash
git add -A
git commit -m "chore(browserx): formatting and lint fixes"
```
