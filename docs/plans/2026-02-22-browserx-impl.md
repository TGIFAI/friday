# Browserx Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a stealth browser tool family (`browserx`) to friday using go-rod + go-rod/stealth, with multi-user isolation, cookie persistence, and headful human-in-the-loop support.

**Architecture:** New `internal/agent/tool/browserx/` package. A singleton BrowserManager owns one Chrome process and manages per-user incognito contexts with cookie persistence. Nine tools (open/read/navigate/click/type/select/scroll/close/list) return JPEG screenshots + markdown content. Agent loop modified to support multi-modal tool results so LLM can visually interpret screenshots.

**Tech Stack:** go-rod, go-rod/stealth, eino schema, bytedance/sonic

**Design doc:** `docs/plans/2026-02-22-browserx-design.md`

---

### Task 1: Add go-rod Dependencies

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`

**Step 1: Add dependencies**

Run:
```bash
cd /Users/dave/go/src/github.com/tgifai/friday
go get github.com/go-rod/rod@latest
go get github.com/go-rod/stealth@latest
```

**Step 2: Verify**

Run: `go mod tidy`
Expected: Clean exit, `go.mod` lists both packages.

**Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "deps: add go-rod and go-rod/stealth for browserx"
```

---

### Task 2: Multi-Modal Tool Results in Agent Loop

The agent loop (`loop.go`) currently converts all tool results to JSON strings. For browserx screenshots to be visible to the LLM as vision input, we need a `MultiModalResult` type that loop.go recognizes and converts to `schema.MessageInputPart` image parts.

**Files:**
- Modify: `internal/agent/tool/registry.go` (add type)
- Modify: `internal/agent/loop.go` (handle multi-modal)
- Test: `internal/agent/loop_test.go` (if exists, add test; otherwise test manually)

**Step 1: Add MultiModalResult type to registry.go**

After the `Tool` interface definition at the bottom of `internal/agent/tool/registry.go`, add:

```go
// MultiModalResult allows tools to return text + images.
// When the agent loop encounters this type, it builds a multi-content
// message so the LLM can visually interpret images (e.g. screenshots).
type MultiModalResult struct {
	Text   string  // JSON-encoded text result (same as normal tool result)
	Images []Image // optional images (e.g. browser screenshots)
}

// Image holds raw image bytes for multi-modal tool results.
type Image struct {
	Data     []byte // raw image bytes (e.g. JPEG)
	MIMEType string // e.g. "image/jpeg"
}
```

**Step 2: Modify loop.go tool result handling**

In `internal/agent/loop.go`, find the tool result construction block (after `res, callErr := ag.tools.ExecuteToolCall(ctx, &call)`). Replace the `else` branch that does `sonic.MarshalString(res)` with multi-modal detection:

Current code (approximately):
```go
if callErr != nil {
    logs.CtxWarn(ctx, "agent tool call failed: %s", callErr)
    resMsg.Content = "ERROR: " + callErr.Error()
} else {
    jsonStr, marshalErr := sonic.MarshalString(res)
    if marshalErr != nil || jsonStr == "" {
        resMsg.Content = "{}"
    } else {
        resMsg.Content = jsonStr
    }
}
```

Replace the `else` branch with:

```go
} else if mmr, ok := res.(*tool.MultiModalResult); ok {
    // Multi-modal result: build content parts with text + images.
    parts := []schema.MessageInputPart{
        {Type: schema.ChatMessagePartTypeText, Text: mmr.Text},
    }
    for _, img := range mmr.Images {
        b64 := base64.StdEncoding.EncodeToString(img.Data)
        parts = append(parts, schema.MessageInputPart{
            Type: schema.ChatMessagePartTypeImageURL,
            Image: &schema.MessageInputImage{
                MessagePartCommon: schema.MessagePartCommon{
                    Base64Data: &b64,
                    MIMEType:   img.MIMEType,
                },
                Detail: schema.ImageURLDetailAuto,
            },
        })
    }
    resMsg.UserInputMultiContent = parts
} else {
    jsonStr, marshalErr := sonic.MarshalString(res)
    if marshalErr != nil || jsonStr == "" {
        resMsg.Content = "{}"
    } else {
        resMsg.Content = jsonStr
    }
}
```

Make sure `encoding/base64` is imported in loop.go (check if it's already imported from the existing multi-modal code in agent.go — if not, add it).

**Step 3: Verify build**

Run: `go build ./...`
Expected: Clean build.

**Step 4: Commit**

```bash
git add internal/agent/tool/registry.go internal/agent/loop.go
git commit -m "feat(agent): add multi-modal tool result support for vision-enabled tools"
```

---

### Task 3: Display Detection

**Files:**
- Create: `internal/agent/tool/browserx/display.go`
- Create: `internal/agent/tool/browserx/display_test.go`

**Step 1: Write the test**

```go
package browserx

import (
	"runtime"
	"testing"
)

func TestHasDisplay(t *testing.T) {
	got, reason := HasDisplay()
	t.Logf("HasDisplay() = %v, reason = %q (os=%s)", got, reason, runtime.GOOS)
	// We can't assert the value (depends on CI vs local), but we can assert:
	// - reason is non-empty when false
	// - reason is empty when true
	if !got && reason == "" {
		t.Error("HasDisplay() returned false but reason is empty")
	}
	if got && reason != "" {
		t.Error("HasDisplay() returned true but reason is non-empty")
	}
}

func TestDisplayErrorMessage(t *testing.T) {
	msg := NoDisplayError()
	if msg == "" {
		t.Error("NoDisplayError() should return a non-empty string")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/agent/tool/browserx/... -run TestHasDisplay -v`
Expected: FAIL (package doesn't exist yet)

**Step 3: Write implementation**

`internal/agent/tool/browserx/display.go`:

```go
package browserx

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// HasDisplay checks whether a desktop environment is available for headful browser mode.
// Returns (true, "") if display is available, or (false, reason) if not.
func HasDisplay() (bool, string) {
	switch runtime.GOOS {
	case "darwin":
		// macOS: check if WindowServer process is running (absent in pure SSH).
		out, err := exec.Command("pgrep", "-x", "WindowServer").Output()
		if err != nil || strings.TrimSpace(string(out)) == "" {
			return false, "WindowServer not running (no GUI session)"
		}
		return true, ""

	case "linux":
		if os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != "" {
			return true, ""
		}
		return false, "neither DISPLAY nor WAYLAND_DISPLAY is set"

	case "windows":
		return true, ""

	default:
		return false, "unsupported platform: " + runtime.GOOS
	}
}

// NoDisplayError returns a user-facing error message when headful mode is unavailable.
func NoDisplayError() string {
	_, reason := HasDisplay()
	return "no desktop environment detected (" + reason + "); use headless=true for non-interactive mode, or ensure a display is available for interactive browser sessions"
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/agent/tool/browserx/... -run TestHasDisplay -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/agent/tool/browserx/display.go internal/agent/tool/browserx/display_test.go
git commit -m "feat(browserx): add desktop display detection"
```

---

### Task 4: Cookie Persistence

**Files:**
- Create: `internal/agent/tool/browserx/cookie.go`
- Create: `internal/agent/tool/browserx/cookie_test.go`

**Step 1: Write the test**

```go
package browserx

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-rod/rod/lib/proto"
)

func TestCookieStore_SaveLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cookies.json")
	store := newCookieStore(path)

	cookies := []*proto.NetworkCookie{
		{Name: "session", Value: "abc123", Domain: ".github.com", Path: "/"},
		{Name: "token", Value: "xyz", Domain: ".github.com", Path: "/api"},
	}

	if err := store.Save(cookies); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(loaded) != 2 {
		t.Fatalf("expected 2 cookies, got %d", len(loaded))
	}
	if loaded[0].Name != "session" || loaded[0].Value != "abc123" {
		t.Errorf("cookie[0] = %+v, want session=abc123", loaded[0])
	}
}

func TestCookieStore_LoadMissing(t *testing.T) {
	store := newCookieStore(filepath.Join(t.TempDir(), "nonexistent.json"))
	cookies, err := store.Load()
	if err != nil {
		t.Fatalf("Load missing file should not error: %v", err)
	}
	if len(cookies) != 0 {
		t.Errorf("expected empty slice, got %d cookies", len(cookies))
	}
}

func TestCookieStore_LoadCorrupted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cookies.json")
	os.WriteFile(path, []byte("not json{{{"), 0644)

	store := newCookieStore(path)
	_, err := store.Load()
	if err == nil {
		t.Error("expected error for corrupted file")
	}
}

func TestCookieStore_AtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cookies.json")
	store := newCookieStore(path)

	cookies := []*proto.NetworkCookie{
		{Name: "a", Value: "1", Domain: ".example.com", Path: "/"},
	}
	if err := store.Save(cookies); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Verify no .tmp file left behind.
	matches, _ := filepath.Glob(filepath.Join(dir, "*.tmp"))
	if len(matches) > 0 {
		t.Errorf("temp file left behind: %v", matches)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/agent/tool/browserx/... -run TestCookieStore -v`
Expected: FAIL

**Step 3: Write implementation**

`internal/agent/tool/browserx/cookie.go`:

```go
package browserx

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/bytedance/sonic"
	"github.com/go-rod/rod/lib/proto"
)

// cookieStore persists browser cookies to a JSON file for session restoration.
type cookieStore struct {
	path string
}

func newCookieStore(path string) *cookieStore {
	return &cookieStore{path: path}
}

// Save writes cookies to disk atomically (write to .tmp then rename).
func (s *cookieStore) Save(cookies []*proto.NetworkCookie) error {
	data, err := sonic.MarshalIndent(cookies, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal cookies: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return fmt.Errorf("create cookie dir: %w", err)
	}

	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("write temp cookie file: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename cookie file: %w", err)
	}
	return nil
}

// Load reads cookies from disk. Returns empty slice if file does not exist.
func (s *cookieStore) Load() ([]*proto.NetworkCookie, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read cookie file: %w", err)
	}

	var cookies []*proto.NetworkCookie
	if err := sonic.Unmarshal(data, &cookies); err != nil {
		return nil, fmt.Errorf("unmarshal cookies: %w", err)
	}
	return cookies, nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/agent/tool/browserx/... -run TestCookieStore -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/agent/tool/browserx/cookie.go internal/agent/tool/browserx/cookie_test.go
git commit -m "feat(browserx): add per-user cookie persistence"
```

---

### Task 5: Selector Resolution

**Files:**
- Create: `internal/agent/tool/browserx/selector.go`
- Create: `internal/agent/tool/browserx/selector_test.go`

**Step 1: Write the test**

```go
package browserx

import "testing"

func TestParseSelector(t *testing.T) {
	tests := []struct {
		input    string
		wantMode selectorMode
		wantVal  string
	}{
		{"button#login", modeCSS, "button#login"},
		{"text=Sign In", modeText, "Sign In"},
		{"xpath=//button", modeXPath, "//button"},
		{"coord=350,420", modeCoord, "350,420"},
		{".class-name", modeCSS, ".class-name"},
		{"text=", modeText, ""},
	}

	for _, tt := range tests {
		mode, val := parseSelector(tt.input)
		if mode != tt.wantMode || val != tt.wantVal {
			t.Errorf("parseSelector(%q) = (%v, %q), want (%v, %q)",
				tt.input, mode, val, tt.wantMode, tt.wantVal)
		}
	}
}

func TestParseCoord(t *testing.T) {
	x, y, err := parseCoord("350,420")
	if err != nil {
		t.Fatalf("parseCoord: %v", err)
	}
	if x != 350 || y != 420 {
		t.Errorf("got (%d, %d), want (350, 420)", x, y)
	}

	_, _, err = parseCoord("invalid")
	if err == nil {
		t.Error("expected error for invalid coord")
	}

	_, _, err = parseCoord("10")
	if err == nil {
		t.Error("expected error for single value")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/agent/tool/browserx/... -run TestParseSelector -v`
Expected: FAIL

**Step 3: Write implementation**

`internal/agent/tool/browserx/selector.go`:

```go
package browserx

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

type selectorMode int

const (
	modeCSS   selectorMode = iota
	modeText               // text=...
	modeXPath              // xpath=...
	modeCoord              // coord=x,y
)

const elementTimeout = 5 * time.Second

// parseSelector extracts the selector mode and value from a selector string.
func parseSelector(s string) (selectorMode, string) {
	switch {
	case strings.HasPrefix(s, "text="):
		return modeText, strings.TrimPrefix(s, "text=")
	case strings.HasPrefix(s, "xpath="):
		return modeXPath, strings.TrimPrefix(s, "xpath=")
	case strings.HasPrefix(s, "coord="):
		return modeCoord, strings.TrimPrefix(s, "coord=")
	default:
		return modeCSS, s
	}
}

// parseCoord parses "x,y" into integer coordinates.
func parseCoord(s string) (int, int, error) {
	parts := strings.SplitN(s, ",", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid coord format %q, expected x,y", s)
	}
	x, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, fmt.Errorf("invalid x coordinate: %w", err)
	}
	y, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, 0, fmt.Errorf("invalid y coordinate: %w", err)
	}
	return x, y, nil
}

// resolveElement finds a DOM element on the page using the given selector string.
// Supports CSS selectors (default), text= prefix, xpath= prefix, and coord= prefix.
func resolveElement(ctx context.Context, page *rod.Page, selector string) (*rod.Element, error) {
	mode, val := parseSelector(selector)

	switch mode {
	case modeText:
		if val == "" {
			return nil, fmt.Errorf("text selector cannot be empty")
		}
		return page.Context(ctx).Timeout(elementTimeout).ElementR("*", val)

	case modeXPath:
		return page.Context(ctx).Timeout(elementTimeout).ElementX(val)

	case modeCoord:
		x, y, err := parseCoord(val)
		if err != nil {
			return nil, err
		}
		el, err := page.Context(ctx).Timeout(elementTimeout).ElementFromPoint(x, y)
		if err != nil {
			return nil, fmt.Errorf("no element at coord (%d, %d): %w", x, y, err)
		}
		return el, nil

	default:
		return page.Context(ctx).Timeout(elementTimeout).Element(val)
	}
}

// scrollPage scrolls the page by one viewport height in the given direction.
func scrollPage(page *rod.Page, direction string) error {
	switch strings.ToLower(direction) {
	case "down":
		return page.Mouse.Scroll(0, 600, 1)
	case "up":
		return page.Mouse.Scroll(0, -600, 1)
	default:
		return fmt.Errorf("invalid scroll direction %q, use 'up' or 'down'", direction)
	}
}

// selectOption selects a dropdown option by value or visible text.
func selectOption(el *rod.Element, value string) error {
	return el.Select([]string{value}, true, rod.SelectorTypeText)
}

// clearAndType clears an input field and types new text.
func clearAndType(el *rod.Element, text string, clear bool) error {
	if clear {
		if err := el.SelectAllText(); err != nil {
			return err
		}
		if err := el.Input(""); err != nil {
			return err
		}
	}
	return el.Input(text)
}

// clickElement scrolls the element into view and clicks it.
func clickElement(el *rod.Element) error {
	if err := el.ScrollIntoView(); err != nil {
		// Ignore scroll errors — element might already be in view.
		_ = err
	}
	return el.Click(proto.InputMouseButtonLeft, 1)
}
```

**Step 4: Run tests**

Run: `go test ./internal/agent/tool/browserx/... -run "TestParseSelector|TestParseCoord" -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/agent/tool/browserx/selector.go internal/agent/tool/browserx/selector_test.go
git commit -m "feat(browserx): add multi-mode selector resolution (CSS/text/xpath/coord)"
```

---

### Task 6: BrowserResult Type + Screenshot Helper

**Files:**
- Create: `internal/agent/tool/browserx/result.go`

**Step 1: Write implementation**

`internal/agent/tool/browserx/result.go`:

```go
package browserx

import (
	"fmt"
	"net/url"
	"time"

	"github.com/bytedance/sonic"
	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"

	"github.com/tgifai/friday/internal/agent/tool"
)

const (
	screenshotQuality = 80
	stableTimeout     = 10 * time.Second
)

// BrowserResult is the unified response for all browserx tools.
type BrowserResult struct {
	PageID     string `json:"page_id"`
	URL        string `json:"url"`
	Title      string `json:"title"`
	Content    string `json:"content,omitempty"`
	Success    bool   `json:"success"`
	Message    string `json:"message,omitempty"`
}

// captureScreenshot takes a JPEG viewport screenshot of the page.
func captureScreenshot(page *rod.Page) ([]byte, error) {
	quality := screenshotQuality
	data, err := page.Screenshot(true, &proto.PageCaptureScreenshot{
		Format:  proto.PageCaptureScreenshotFormatJpeg,
		Quality: &quality,
	})
	if err != nil {
		return nil, fmt.Errorf("screenshot: %w", err)
	}
	return data, nil
}

// extractPageContent gets the page HTML and converts it to markdown.
func extractPageContent(page *rod.Page) string {
	html, err := page.HTML()
	if err != nil {
		return ""
	}

	pageURL := page.MustInfo().URL
	md, err := htmltomarkdown.ConvertString(html)
	if err != nil {
		return html
	}

	// Prepend title if available.
	title, _ := pageTitle(page)
	if title != "" {
		md = "# " + title + "\n\n" + md
	}

	// Truncate to keep token count reasonable.
	const maxChars = 50000
	if len(md) > maxChars {
		md = md[:maxChars]
	}

	_ = pageURL // reserved for future URL-based extraction
	return md
}

// pageTitle returns the page's document title.
func pageTitle(page *rod.Page) (string, error) {
	info, err := page.Info()
	if err != nil {
		return "", err
	}
	return info.Title, nil
}

// buildResult builds a BrowserResult for a page and wraps it as a MultiModalResult
// with an optional screenshot.
func buildResult(page *rod.Page, pageID string, includeContent bool, message string) (*tool.MultiModalResult, error) {
	title, _ := pageTitle(page)
	info, _ := page.Info()

	result := BrowserResult{
		PageID:  pageID,
		URL:     info.URL,
		Title:   title,
		Success: true,
		Message: message,
	}

	if includeContent {
		result.Content = extractPageContent(page)
	}

	text, err := sonic.MarshalString(result)
	if err != nil {
		return nil, fmt.Errorf("marshal result: %w", err)
	}

	mmr := &tool.MultiModalResult{Text: text}

	// Capture screenshot.
	if screenshot, err := captureScreenshot(page); err == nil {
		mmr.Images = []tool.Image{{Data: screenshot, MIMEType: "image/jpeg"}}
	}

	return mmr, nil
}

// buildErrorResult builds a failure result (no screenshot).
func buildErrorResult(pageID, message string) *tool.MultiModalResult {
	result := BrowserResult{
		PageID:  pageID,
		Success: false,
		Message: message,
	}
	text, _ := sonic.MarshalString(result)
	return &tool.MultiModalResult{Text: text}
}

// waitStable waits for the page DOM to stabilize, with a timeout fallback.
func waitStable(page *rod.Page) {
	_ = rod.Try(func() {
		page.Timeout(stableTimeout).MustWaitStable()
	})
}

// waitLoad waits for page load event, with a timeout fallback.
func waitLoad(page *rod.Page) {
	_ = rod.Try(func() {
		page.Timeout(stableTimeout).MustWaitLoad()
	})
}

// parseURLArg validates and parses a URL string from tool args.
func parseURLArg(rawURL string) (*url.URL, error) {
	if rawURL == "" {
		return nil, fmt.Errorf("url is required")
	}
	parsed, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("only http and https URLs are allowed")
	}
	return parsed, nil
}
```

**Step 2: Verify build**

Run: `go build ./internal/agent/tool/browserx/...`
Expected: Clean build

**Step 3: Commit**

```bash
git add internal/agent/tool/browserx/result.go
git commit -m "feat(browserx): add BrowserResult type and screenshot/content helpers"
```

---

### Task 7: Stealth Configuration

**Files:**
- Create: `internal/agent/tool/browserx/stealth.go`

**Step 1: Write implementation**

`internal/agent/tool/browserx/stealth.go`:

```go
package browserx

import (
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/stealth"
)

const (
	defaultViewportWidth  = 1280
	defaultViewportHeight = 800
)

// launchBrowser starts a Chrome browser with stealth flags.
// headless=true uses --headless=new (near-perfect fingerprint).
// headless=false opens a visible browser window (requires display).
func launchBrowser(headless bool) (*rod.Browser, error) {
	l := launcher.New().
		Set("disable-blink-features", "AutomationControlled").
		Set("disable-infobars").
		Set("no-first-run").
		Set("no-default-browser-check")

	if headless {
		l = l.Headless(true)
	} else {
		l = l.Headless(false)
	}

	controlURL, err := l.Launch()
	if err != nil {
		return nil, err
	}

	browser := rod.New().ControlURL(controlURL)
	if err := browser.Connect(); err != nil {
		return nil, err
	}

	return browser, nil
}

// newStealthPage creates a new page with stealth evasions applied.
// Uses go-rod/stealth to inject puppeteer-extra-plugin-stealth JS.
func newStealthPage(browser *rod.Browser) (*rod.Page, error) {
	page, err := stealth.Page(browser)
	if err != nil {
		return nil, err
	}

	// Set viewport size.
	if err := page.SetViewport(&rod.ViewportParams{
		Width:  defaultViewportWidth,
		Height: defaultViewportHeight,
	}); err != nil {
		return nil, err
	}

	// Auto-accept dialogs to prevent blocking.
	go page.EachEvent(func(e *rod.DialogEvent) {
		_ = page.HandleDialog(true, "")
	})()

	return page, nil
}
```

Note: The dialog handling API may differ slightly between rod versions. The implementer should check the rod docs for `page.EachEvent` and `proto.PageJavascriptDialogOpening`. If the above pattern doesn't compile, use:

```go
import "github.com/go-rod/rod/lib/proto"

go page.EachEvent(func(e *proto.PageJavascriptDialogOpening) {
    _ = proto.PageHandleJavaScriptDialog{Accept: true}.Call(page)
})()
```

**Step 2: Verify build**

Run: `go build ./internal/agent/tool/browserx/...`
Expected: Clean build

**Step 3: Commit**

```bash
git add internal/agent/tool/browserx/stealth.go
git commit -m "feat(browserx): add stealth browser/page launcher with anti-detection"
```

---

### Task 8: BrowserManager

This is the most complex piece — manages browser lifecycle, per-user sessions, page tracking, and idle cleanup.

**Files:**
- Create: `internal/agent/tool/browserx/manager.go`
- Create: `internal/agent/tool/browserx/manager_test.go`

**Step 1: Write the test**

```go
package browserx

import (
	"context"
	"testing"

	"github.com/tgifai/friday/internal/consts"
)

func TestUserKeyFromContext(t *testing.T) {
	ctx := context.Background()
	ctx = context.WithValue(ctx, consts.CtxKeyChannelID, "telegram-main")
	ctx = context.WithValue(ctx, consts.CtxKeyChatID, "12345")

	key := userKeyFromContext(ctx)
	if key != "telegram-main:12345" {
		t.Errorf("got %q, want %q", key, "telegram-main:12345")
	}
}

func TestUserKeyFromContext_Empty(t *testing.T) {
	ctx := context.Background()
	key := userKeyFromContext(ctx)
	if key != ":" {
		t.Errorf("got %q, want %q", key, ":")
	}
}

func TestSanitizeUserKey(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"telegram-main:12345", "telegram-main_12345"},
		{"lark:group/abc", "lark_group_abc"},
		{"simple", "simple"},
	}
	for _, tt := range tests {
		got := sanitizeUserKey(tt.input)
		if got != tt.want {
			t.Errorf("sanitizeUserKey(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/agent/tool/browserx/... -run TestUserKey -v`
Expected: FAIL

**Step 3: Write implementation**

`internal/agent/tool/browserx/manager.go`:

```go
package browserx

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/google/uuid"

	"github.com/tgifai/friday/internal/consts"
	"github.com/tgifai/friday/internal/pkg/logs"
	pkgutils "github.com/tgifai/friday/internal/pkg/utils"
)

const (
	maxPagesPerUser = 5
	maxPagesGlobal  = 20
	pageIdleTimeout = 15 * time.Minute
	sessIdleTimeout = 30 * time.Minute
	cleanupInterval = 1 * time.Minute
)

// PageInfo holds metadata for an open browser page.
type PageInfo struct {
	Page    *rod.Page
	ID      string
	URL     string
	Created time.Time
	LastUse time.Time
}

// UserSession holds an isolated browser context for one user.
type UserSession struct {
	mu      sync.Mutex
	context *rod.Browser // incognito context
	pages   map[string]*PageInfo
	cookies *cookieStore
	lastUse time.Time
}

// Manager is the singleton browser lifecycle manager.
type Manager struct {
	mu       sync.Mutex
	browser  *rod.Browser
	sessions map[string]*UserSession // userKey -> session
	dataDir  string
	headless bool
	stop     chan struct{}
}

var (
	mgrOnce sync.Once
	mgrInst *Manager
)

// GetManager returns the singleton BrowserManager, creating it on first call.
// dataDir is typically ~/.friday/browser-data/.
func GetManager(dataDir string) *Manager {
	mgrOnce.Do(func() {
		mgrInst = &Manager{
			sessions: make(map[string]*UserSession),
			dataDir:  dataDir,
			stop:     make(chan struct{}),
		}
		go mgrInst.cleanupLoop()
	})
	return mgrInst
}

// ensureBrowser lazily starts the Chrome process.
func (m *Manager) ensureBrowser(headless bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.browser != nil {
		return nil
	}

	if !headless {
		if ok, _ := HasDisplay(); !ok {
			return fmt.Errorf(NoDisplayError())
		}
	}

	browser, err := launchBrowser(headless)
	if err != nil {
		return fmt.Errorf("launch browser: %w", err)
	}
	m.browser = browser
	m.headless = headless
	return nil
}

// GetSession returns the user's session, creating one if needed.
func (m *Manager) GetSession(ctx context.Context) (*UserSession, error) {
	userKey := userKeyFromContext(ctx)

	m.mu.Lock()
	sess, exists := m.sessions[userKey]
	if exists {
		m.mu.Unlock()
		sess.mu.Lock()
		sess.lastUse = time.Now()
		sess.mu.Unlock()
		return sess, nil
	}

	if m.browser == nil {
		m.mu.Unlock()
		return nil, fmt.Errorf("browser not started; call OpenPage first")
	}

	// Create incognito context for user isolation.
	incognito, err := m.browser.Incognito()
	if err != nil {
		m.mu.Unlock()
		return nil, fmt.Errorf("create incognito context: %w", err)
	}

	cookiePath := filepath.Join(m.dataDir, sanitizeUserKey(userKey), "cookies.json")
	sess = &UserSession{
		context: incognito,
		pages:   make(map[string]*PageInfo),
		cookies: newCookieStore(cookiePath),
		lastUse: time.Now(),
	}

	// Load persisted cookies.
	if cookies, err := sess.cookies.Load(); err != nil {
		logs.Warn("[browserx] load cookies for %s: %v", userKey, err)
	} else if len(cookies) > 0 {
		for _, c := range cookies {
			_ = incognito.SetCookies([]*rod.CookieParam{{
				Name:     c.Name,
				Value:    c.Value,
				Domain:   c.Domain,
				Path:     c.Path,
				Expires:  c.Expires,
				HTTPOnly: c.HTTPOnly,
				Secure:   c.Secure,
				SameSite: c.SameSite,
			}})
		}
		logs.Info("[browserx] restored %d cookies for %s", len(cookies), userKey)
	}

	m.sessions[userKey] = sess
	m.mu.Unlock()
	return sess, nil
}

// OpenPage creates a new stealth page in the user's session.
func (m *Manager) OpenPage(ctx context.Context, targetURL string, headless bool) (*PageInfo, error) {
	// Validate URL (SSRF check).
	parsed, err := parseURLArg(targetURL)
	if err != nil {
		return nil, err
	}
	if pkgutils.IsPrivateHost(parsed.Hostname()) {
		return nil, fmt.Errorf("access to private/internal addresses is not allowed")
	}

	// Ensure browser is running.
	if err := m.ensureBrowser(headless); err != nil {
		return nil, err
	}

	sess, err := m.GetSession(ctx)
	if err != nil {
		return nil, err
	}

	sess.mu.Lock()
	defer sess.mu.Unlock()

	// Check page limits.
	if len(sess.pages) >= maxPagesPerUser {
		return nil, fmt.Errorf("page limit reached (%d per user)", maxPagesPerUser)
	}
	if m.totalPages() >= maxPagesGlobal {
		return nil, fmt.Errorf("global page limit reached (%d)", maxPagesGlobal)
	}

	// Create stealth page.
	page, err := newStealthPage(sess.context)
	if err != nil {
		return nil, fmt.Errorf("create page: %w", err)
	}

	// Navigate.
	if err := page.Navigate(targetURL); err != nil {
		page.Close()
		return nil, fmt.Errorf("navigate to %s: %w", targetURL, err)
	}
	waitLoad(page)
	waitStable(page)

	// Save cookies after navigation.
	m.saveCookies(sess)

	pageID := uuid.New().String()[:8]
	info := &PageInfo{
		Page:    page,
		ID:      pageID,
		URL:     targetURL,
		Created: time.Now(),
		LastUse: time.Now(),
	}
	sess.pages[pageID] = info
	return info, nil
}

// GetPage returns a page by ID, or the most recently used page if id is empty.
func (m *Manager) GetPage(ctx context.Context, pageID string) (*PageInfo, error) {
	sess, err := m.GetSession(ctx)
	if err != nil {
		return nil, err
	}

	sess.mu.Lock()
	defer sess.mu.Unlock()

	if pageID != "" {
		p, ok := sess.pages[pageID]
		if !ok {
			return nil, fmt.Errorf("page %q not found", pageID)
		}
		p.LastUse = time.Now()
		return p, nil
	}

	// Find most recently used page.
	var latest *PageInfo
	for _, p := range sess.pages {
		if latest == nil || p.LastUse.After(latest.LastUse) {
			latest = p
		}
	}
	if latest == nil {
		return nil, fmt.Errorf("no open pages; use browser_open first")
	}
	latest.LastUse = time.Now()
	return latest, nil
}

// ClosePage closes a specific page or all pages for the user.
func (m *Manager) ClosePage(ctx context.Context, pageID string) ([]string, error) {
	sess, err := m.GetSession(ctx)
	if err != nil {
		return nil, err
	}

	sess.mu.Lock()
	defer sess.mu.Unlock()

	var closed []string
	if pageID != "" {
		p, ok := sess.pages[pageID]
		if !ok {
			return nil, fmt.Errorf("page %q not found", pageID)
		}
		p.Page.Close()
		delete(sess.pages, pageID)
		closed = append(closed, pageID)
	} else {
		for id, p := range sess.pages {
			p.Page.Close()
			delete(sess.pages, id)
			closed = append(closed, id)
		}
	}

	m.saveCookies(sess)
	return closed, nil
}

// ListPages returns metadata for all open pages of the current user.
func (m *Manager) ListPages(ctx context.Context) ([]map[string]string, error) {
	sess, err := m.GetSession(ctx)
	if err != nil {
		// No session = no pages.
		return nil, nil
	}

	sess.mu.Lock()
	defer sess.mu.Unlock()

	result := make([]map[string]string, 0, len(sess.pages))
	for _, p := range sess.pages {
		info, _ := p.Page.Info()
		url := p.URL
		title := ""
		if info != nil {
			url = info.URL
			title = info.Title
		}
		result = append(result, map[string]string{
			"page_id":    p.ID,
			"url":        url,
			"title":      title,
			"created_at": p.Created.Format(time.RFC3339),
		})
	}
	return result, nil
}

// Close shuts down all sessions and the browser process.
func (m *Manager) Close() {
	close(m.stop)
	m.mu.Lock()
	defer m.mu.Unlock()

	for key, sess := range m.sessions {
		sess.mu.Lock()
		m.saveCookies(sess)
		for _, p := range sess.pages {
			p.Page.Close()
		}
		sess.context.Close()
		sess.mu.Unlock()
		delete(m.sessions, key)
	}

	if m.browser != nil {
		m.browser.Close()
		m.browser = nil
	}
}

// saveCookies persists cookies for a session. Caller must hold sess.mu.
func (m *Manager) saveCookies(sess *UserSession) {
	cookies, err := sess.context.GetCookies()
	if err != nil {
		logs.Warn("[browserx] get cookies: %v", err)
		return
	}
	if err := sess.cookies.Save(cookies); err != nil {
		logs.Warn("[browserx] save cookies: %v", err)
	}
}

// totalPages counts all open pages across all sessions.
func (m *Manager) totalPages() int {
	total := 0
	for _, sess := range m.sessions {
		total += len(sess.pages)
	}
	return total
}

// cleanupLoop periodically evicts idle pages and sessions.
func (m *Manager) cleanupLoop() {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.stop:
			return
		case <-ticker.C:
			m.cleanup()
		}
	}
}

func (m *Manager) cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for key, sess := range m.sessions {
		sess.mu.Lock()

		// Evict idle pages.
		for id, p := range sess.pages {
			if now.Sub(p.LastUse) > pageIdleTimeout {
				logs.Info("[browserx] closing idle page %s for %s", id, key)
				p.Page.Close()
				delete(sess.pages, id)
			}
		}

		// Evict idle sessions (no pages left for a while).
		if len(sess.pages) == 0 && now.Sub(sess.lastUse) > sessIdleTimeout {
			logs.Info("[browserx] closing idle session for %s", key)
			m.saveCookies(sess)
			sess.context.Close()
			sess.mu.Unlock()
			delete(m.sessions, key)
			continue
		}

		sess.mu.Unlock()
	}
}

// --- helpers ---

func userKeyFromContext(ctx context.Context) string {
	channelID, _ := ctx.Value(consts.CtxKeyChannelID).(string)
	chatID, _ := ctx.Value(consts.CtxKeyChatID).(string)
	return channelID + ":" + chatID
}

var unsafeChars = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

func sanitizeUserKey(key string) string {
	return unsafeChars.ReplaceAllString(key, "_")
}
```

**Important implementation note:** The `browser.GetCookies()` and `browser.SetCookies()` APIs may differ between rod versions. The implementer should check the rod documentation for the incognito browser context. If `GetCookies` is on `*rod.Browser`, use it directly. If it's page-level only, iterate pages to collect cookies.

**Step 4: Run tests**

Run: `go test ./internal/agent/tool/browserx/... -run TestUserKey -v`
Expected: PASS

Run: `go test ./internal/agent/tool/browserx/... -run TestSanitize -v`
Expected: PASS

**Step 5: Verify build**

Run: `go build ./internal/agent/tool/browserx/...`
Expected: Clean build

**Step 6: Commit**

```bash
git add internal/agent/tool/browserx/manager.go internal/agent/tool/browserx/manager_test.go
git commit -m "feat(browserx): add BrowserManager with multi-user sessions and lifecycle management"
```

---

### Task 9: Navigation Tools (browser_open, browser_read, browser_navigate)

These three tools follow the same pattern and share helpers from `result.go` and `manager.go`.

**Files:**
- Create: `internal/agent/tool/browserx/tool_open.go`
- Create: `internal/agent/tool/browserx/tool_read.go`
- Create: `internal/agent/tool/browserx/tool_navigate.go`
- Create: `internal/agent/tool/browserx/tools.go` (shared registration helper + manager accessor)

**Step 1: Write tools.go (shared infrastructure)**

`internal/agent/tool/browserx/tools.go`:

```go
package browserx

import (
	"os"
	"path/filepath"

	"github.com/tgifai/friday/internal/agent/tool"
	"github.com/tgifai/friday/internal/pkg/logs"
)

// defaultDataDir returns ~/.friday/browser-data/.
func defaultDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		logs.Warn("[browserx] cannot determine home dir: %v", err)
		return filepath.Join(os.TempDir(), "friday-browser-data")
	}
	return filepath.Join(home, ".friday", "browser-data")
}

// manager returns the singleton BrowserManager.
func manager() *Manager {
	return GetManager(defaultDataDir())
}

// NewTools returns all browserx tools for registration.
func NewTools() []tool.Tool {
	return []tool.Tool{
		&OpenTool{},
		&ReadTool{},
		&NavigateTool{},
		&ClickTool{},
		&TypeTool{},
		&SelectTool{},
		&ScrollTool{},
		&CloseTool{},
		&ListTool{},
	}
}
```

**Step 2: Write tool_open.go**

```go
package browserx

import (
	"context"

	"github.com/cloudwego/eino/schema"

	"github.com/tgifai/friday/internal/agent/tool"
)

type OpenTool struct{}

func (t *OpenTool) Name() string { return "browser_open" }

func (t *OpenTool) Description() string {
	return "Open a URL in a browser with anti-detection stealth. Returns a screenshot and page content as markdown. Use headless=false (default) for interactive sessions where a human may need to interact with the browser window (e.g. login). Use headless=true for non-interactive content extraction."
}

func (t *OpenTool) ToolInfo() *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: t.Name(),
		Desc: t.Description(),
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"url": {
				Type:     schema.String,
				Desc:     "The URL to open (must be http or https)",
				Required: true,
			},
			"headless": {
				Type: schema.Boolean,
				Desc: "Run in headless mode without visible window (default: false)",
			},
		}),
	}
}

func (t *OpenTool) Execute(ctx context.Context, args map[string]any) (any, error) {
	rawURL, _ := args["url"].(string)
	headless, _ := args["headless"].(bool)

	pageInfo, err := manager().OpenPage(ctx, rawURL, headless)
	if err != nil {
		return nil, err
	}

	return buildResult(pageInfo.Page, pageInfo.ID, true, "page opened successfully")
}
```

**Step 3: Write tool_read.go**

```go
package browserx

import (
	"context"

	"github.com/cloudwego/eino/schema"
)

type ReadTool struct{}

func (t *ReadTool) Name() string { return "browser_read" }

func (t *ReadTool) Description() string {
	return "Read the current state of an open browser page. Returns a screenshot and page content as markdown. Use this after a human has interacted with the browser to check the current state."
}

func (t *ReadTool) ToolInfo() *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: t.Name(),
		Desc: t.Description(),
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"page_id": {
				Type: schema.String,
				Desc: "ID of the page to read (default: most recently used page)",
			},
		}),
	}
}

func (t *ReadTool) Execute(ctx context.Context, args map[string]any) (any, error) {
	pageID, _ := args["page_id"].(string)

	pageInfo, err := manager().GetPage(ctx, pageID)
	if err != nil {
		return nil, err
	}

	// Save cookies (user may have logged in).
	sess, _ := manager().GetSession(ctx)
	if sess != nil {
		sess.mu.Lock()
		manager().saveCookies(sess)
		sess.mu.Unlock()
	}

	return buildResult(pageInfo.Page, pageInfo.ID, true, "")
}
```

**Step 4: Write tool_navigate.go**

```go
package browserx

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/schema"

	pkgutils "github.com/tgifai/friday/internal/pkg/utils"
)

type NavigateTool struct{}

func (t *NavigateTool) Name() string { return "browser_navigate" }

func (t *NavigateTool) Description() string {
	return "Navigate an open browser page to a new URL. Preserves cookies and login state. Returns screenshot and content."
}

func (t *NavigateTool) ToolInfo() *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: t.Name(),
		Desc: t.Description(),
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"url": {
				Type:     schema.String,
				Desc:     "The URL to navigate to",
				Required: true,
			},
			"page_id": {
				Type: schema.String,
				Desc: "ID of the page to navigate (default: most recently used page)",
			},
		}),
	}
}

func (t *NavigateTool) Execute(ctx context.Context, args map[string]any) (any, error) {
	rawURL, _ := args["url"].(string)
	pageID, _ := args["page_id"].(string)

	parsed, err := parseURLArg(rawURL)
	if err != nil {
		return nil, err
	}
	if pkgutils.IsPrivateHost(parsed.Hostname()) {
		return nil, fmt.Errorf("access to private/internal addresses is not allowed")
	}

	pageInfo, err := manager().GetPage(ctx, pageID)
	if err != nil {
		return nil, err
	}

	if err := pageInfo.Page.Navigate(rawURL); err != nil {
		return nil, fmt.Errorf("navigate: %w", err)
	}
	waitLoad(pageInfo.Page)
	waitStable(pageInfo.Page)

	// Update URL and save cookies.
	pageInfo.URL = rawURL
	sess, _ := manager().GetSession(ctx)
	if sess != nil {
		sess.mu.Lock()
		manager().saveCookies(sess)
		sess.mu.Unlock()
	}

	return buildResult(pageInfo.Page, pageInfo.ID, true, "navigated successfully")
}
```

**Step 5: Verify build**

Run: `go build ./internal/agent/tool/browserx/...`
Expected: Clean build

**Step 6: Commit**

```bash
git add internal/agent/tool/browserx/tools.go \
        internal/agent/tool/browserx/tool_open.go \
        internal/agent/tool/browserx/tool_read.go \
        internal/agent/tool/browserx/tool_navigate.go
git commit -m "feat(browserx): add browser_open, browser_read, browser_navigate tools"
```

---

### Task 10: Interaction Tools (browser_click, browser_type, browser_select, browser_scroll)

**Files:**
- Create: `internal/agent/tool/browserx/tool_click.go`
- Create: `internal/agent/tool/browserx/tool_type.go`
- Create: `internal/agent/tool/browserx/tool_select.go`
- Create: `internal/agent/tool/browserx/tool_scroll.go`

**Step 1: Write tool_click.go**

```go
package browserx

import (
	"context"

	"github.com/cloudwego/eino/schema"
)

type ClickTool struct{}

func (t *ClickTool) Name() string { return "browser_click" }

func (t *ClickTool) Description() string {
	return "Click an element on the page. Supports CSS selectors (default), text matching (text=Sign In), XPath (xpath=//button), and coordinates (coord=350,420). Returns a screenshot after clicking."
}

func (t *ClickTool) ToolInfo() *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: t.Name(),
		Desc: t.Description(),
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"selector": {
				Type:     schema.String,
				Desc:     "Element selector: CSS (default), text=..., xpath=..., or coord=x,y",
				Required: true,
			},
			"page_id": {
				Type: schema.String,
				Desc: "ID of the page (default: most recently used page)",
			},
		}),
	}
}

func (t *ClickTool) Execute(ctx context.Context, args map[string]any) (any, error) {
	selector, _ := args["selector"].(string)
	pageID, _ := args["page_id"].(string)

	if selector == "" {
		return nil, fmt.Errorf("selector is required")
	}

	pageInfo, err := manager().GetPage(ctx, pageID)
	if err != nil {
		return nil, err
	}

	el, err := resolveElement(ctx, pageInfo.Page, selector)
	if err != nil {
		return buildErrorResult(pageInfo.ID, "element not found: "+err.Error()), nil
	}

	if err := clickElement(el); err != nil {
		return buildErrorResult(pageInfo.ID, "click failed: "+err.Error()), nil
	}

	waitStable(pageInfo.Page)
	return buildResult(pageInfo.Page, pageInfo.ID, false, "clicked successfully")
}
```

Note: add `"fmt"` to the imports.

**Step 2: Write tool_type.go**

```go
package browserx

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/schema"
)

type TypeTool struct{}

func (t *TypeTool) Name() string { return "browser_type" }

func (t *TypeTool) Description() string {
	return "Type text into an input field. By default clears existing content first. Returns a screenshot after typing."
}

func (t *TypeTool) ToolInfo() *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: t.Name(),
		Desc: t.Description(),
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"selector": {
				Type:     schema.String,
				Desc:     "Element selector for the input field",
				Required: true,
			},
			"text": {
				Type:     schema.String,
				Desc:     "Text to type into the field",
				Required: true,
			},
			"clear": {
				Type: schema.Boolean,
				Desc: "Clear existing content before typing (default: true)",
			},
			"page_id": {
				Type: schema.String,
				Desc: "ID of the page (default: most recently used page)",
			},
		}),
	}
}

func (t *TypeTool) Execute(ctx context.Context, args map[string]any) (any, error) {
	selector, _ := args["selector"].(string)
	text, _ := args["text"].(string)
	pageID, _ := args["page_id"].(string)

	if selector == "" {
		return nil, fmt.Errorf("selector is required")
	}
	if text == "" {
		return nil, fmt.Errorf("text is required")
	}

	// Default clear=true.
	clear := true
	if v, ok := args["clear"].(bool); ok {
		clear = v
	}

	pageInfo, err := manager().GetPage(ctx, pageID)
	if err != nil {
		return nil, err
	}

	el, err := resolveElement(ctx, pageInfo.Page, selector)
	if err != nil {
		return buildErrorResult(pageInfo.ID, "element not found: "+err.Error()), nil
	}

	if err := clearAndType(el, text, clear); err != nil {
		return buildErrorResult(pageInfo.ID, "type failed: "+err.Error()), nil
	}

	// Screenshot does NOT include typed text in result (security: passwords).
	return buildResult(pageInfo.Page, pageInfo.ID, false, "typed successfully")
}
```

**Step 3: Write tool_select.go**

```go
package browserx

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/schema"
)

type SelectTool struct{}

func (t *SelectTool) Name() string { return "browser_select" }

func (t *SelectTool) Description() string {
	return "Select an option from a dropdown. The value parameter matches option text. Returns a screenshot after selection."
}

func (t *SelectTool) ToolInfo() *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: t.Name(),
		Desc: t.Description(),
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"selector": {
				Type:     schema.String,
				Desc:     "CSS selector for the <select> element",
				Required: true,
			},
			"value": {
				Type:     schema.String,
				Desc:     "Option text or value to select",
				Required: true,
			},
			"page_id": {
				Type: schema.String,
				Desc: "ID of the page (default: most recently used page)",
			},
		}),
	}
}

func (t *SelectTool) Execute(ctx context.Context, args map[string]any) (any, error) {
	selector, _ := args["selector"].(string)
	value, _ := args["value"].(string)
	pageID, _ := args["page_id"].(string)

	if selector == "" {
		return nil, fmt.Errorf("selector is required")
	}
	if value == "" {
		return nil, fmt.Errorf("value is required")
	}

	pageInfo, err := manager().GetPage(ctx, pageID)
	if err != nil {
		return nil, err
	}

	el, err := resolveElement(ctx, pageInfo.Page, selector)
	if err != nil {
		return buildErrorResult(pageInfo.ID, "element not found: "+err.Error()), nil
	}

	if err := selectOption(el, value); err != nil {
		return buildErrorResult(pageInfo.ID, "select failed: "+err.Error()), nil
	}

	waitStable(pageInfo.Page)
	return buildResult(pageInfo.Page, pageInfo.ID, false, "selected successfully")
}
```

**Step 4: Write tool_scroll.go**

```go
package browserx

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/schema"
)

type ScrollTool struct{}

func (t *ScrollTool) Name() string { return "browser_scroll" }

func (t *ScrollTool) Description() string {
	return "Scroll the page up or down by one viewport height. Returns a screenshot of the new viewport."
}

func (t *ScrollTool) ToolInfo() *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: t.Name(),
		Desc: t.Description(),
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"direction": {
				Type:     schema.String,
				Desc:     "Scroll direction: 'up' or 'down'",
				Required: true,
			},
			"page_id": {
				Type: schema.String,
				Desc: "ID of the page (default: most recently used page)",
			},
		}),
	}
}

func (t *ScrollTool) Execute(ctx context.Context, args map[string]any) (any, error) {
	direction, _ := args["direction"].(string)
	pageID, _ := args["page_id"].(string)

	if direction == "" {
		return nil, fmt.Errorf("direction is required (up or down)")
	}

	pageInfo, err := manager().GetPage(ctx, pageID)
	if err != nil {
		return nil, err
	}

	if err := scrollPage(pageInfo.Page, direction); err != nil {
		return buildErrorResult(pageInfo.ID, err.Error()), nil
	}

	waitStable(pageInfo.Page)
	return buildResult(pageInfo.Page, pageInfo.ID, false, "scrolled "+direction)
}
```

**Step 5: Verify build**

Run: `go build ./internal/agent/tool/browserx/...`
Expected: Clean build

**Step 6: Commit**

```bash
git add internal/agent/tool/browserx/tool_click.go \
        internal/agent/tool/browserx/tool_type.go \
        internal/agent/tool/browserx/tool_select.go \
        internal/agent/tool/browserx/tool_scroll.go
git commit -m "feat(browserx): add browser_click, browser_type, browser_select, browser_scroll tools"
```

---

### Task 11: Management Tools (browser_close, browser_list)

**Files:**
- Create: `internal/agent/tool/browserx/tool_close.go`
- Create: `internal/agent/tool/browserx/tool_list.go`

**Step 1: Write tool_close.go**

```go
package browserx

import (
	"context"
	"fmt"

	"github.com/bytedance/sonic"
	"github.com/cloudwego/eino/schema"
)

type CloseTool struct{}

func (t *CloseTool) Name() string { return "browser_close" }

func (t *CloseTool) Description() string {
	return "Close a browser page. If no page_id is specified, closes all pages for the current user."
}

func (t *CloseTool) ToolInfo() *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: t.Name(),
		Desc: t.Description(),
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"page_id": {
				Type: schema.String,
				Desc: "ID of the page to close (omit to close all)",
			},
		}),
	}
}

func (t *CloseTool) Execute(ctx context.Context, args map[string]any) (any, error) {
	pageID, _ := args["page_id"].(string)

	closed, err := manager().ClosePage(ctx, pageID)
	if err != nil {
		return nil, err
	}

	result := map[string]any{
		"success":      true,
		"closed_pages": closed,
		"message":      fmt.Sprintf("closed %d page(s)", len(closed)),
	}
	out, _ := sonic.MarshalString(result)
	return out, nil
}
```

**Step 2: Write tool_list.go**

```go
package browserx

import (
	"context"

	"github.com/bytedance/sonic"
	"github.com/cloudwego/eino/schema"
)

type ListTool struct{}

func (t *ListTool) Name() string { return "browser_list" }

func (t *ListTool) Description() string {
	return "List all open browser pages for the current user."
}

func (t *ListTool) ToolInfo() *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: t.Name(),
		Desc: t.Description(),
		ParamsOneOf: schema.NewParamsOneOfByParams(nil),
	}
}

func (t *ListTool) Execute(ctx context.Context, _ map[string]any) (any, error) {
	pages, err := manager().ListPages(ctx)
	if err != nil {
		return nil, err
	}

	result := map[string]any{
		"pages": pages,
		"count": len(pages),
	}
	out, _ := sonic.MarshalString(result)
	return out, nil
}
```

**Step 3: Verify build**

Run: `go build ./internal/agent/tool/browserx/...`
Expected: Clean build

**Step 4: Commit**

```bash
git add internal/agent/tool/browserx/tool_close.go internal/agent/tool/browserx/tool_list.go
git commit -m "feat(browserx): add browser_close and browser_list tools"
```

---

### Task 12: Register browserx Tools in Agent

**Files:**
- Modify: `internal/agent/agent.go`

**Step 1: Add import and registration**

In `internal/agent/agent.go`, add import:

```go
"github.com/tgifai/friday/internal/agent/tool/browserx"
```

In the `Init()` method, after the agent delegation tools block (after `_ = ag.tools.Register(agentx.NewAgentTool(ag.workspace))`), add:

```go
// browser tools
for _, bt := range browserx.NewTools() {
    _ = ag.tools.Register(bt)
}
```

**Step 2: Verify build**

Run: `go build ./...`
Expected: Clean build

**Step 3: Run all tests**

Run: `go test ./... 2>&1 | tail -30`
Expected: All existing tests PASS. browserx tests pass.

**Step 4: Commit**

```bash
git add internal/agent/agent.go
git commit -m "feat: register browserx tools in agent init"
```

---

### Task 13: Integration Test

Write a basic test that verifies the tools are constructable and have correct metadata, without requiring a real Chrome instance.

**Files:**
- Create: `internal/agent/tool/browserx/tools_test.go`

**Step 1: Write test**

```go
package browserx

import (
	"testing"
)

func TestAllToolsMetadata(t *testing.T) {
	tools := NewTools()

	expectedNames := map[string]bool{
		"browser_open":     false,
		"browser_read":     false,
		"browser_navigate": false,
		"browser_click":    false,
		"browser_type":     false,
		"browser_select":   false,
		"browser_scroll":   false,
		"browser_close":    false,
		"browser_list":     false,
	}

	if len(tools) != len(expectedNames) {
		t.Fatalf("expected %d tools, got %d", len(expectedNames), len(tools))
	}

	for _, tool := range tools {
		name := tool.Name()
		if _, ok := expectedNames[name]; !ok {
			t.Errorf("unexpected tool: %s", name)
			continue
		}
		expectedNames[name] = true

		if tool.Description() == "" {
			t.Errorf("tool %s has empty description", name)
		}
		info := tool.ToolInfo()
		if info == nil {
			t.Errorf("tool %s has nil ToolInfo", name)
		}
		if info.Name != name {
			t.Errorf("tool %s: ToolInfo.Name = %q, want %q", name, info.Name, name)
		}
	}

	for name, found := range expectedNames {
		if !found {
			t.Errorf("missing tool: %s", name)
		}
	}
}

func TestDefaultDataDir(t *testing.T) {
	dir := defaultDataDir()
	if dir == "" {
		t.Error("defaultDataDir() returned empty string")
	}
	t.Logf("dataDir = %s", dir)
}
```

**Step 2: Run tests**

Run: `go test ./internal/agent/tool/browserx/... -v`
Expected: All tests PASS

**Step 3: Run full test suite**

Run: `go test ./... 2>&1 | tail -20`
Expected: All tests PASS

**Step 4: Commit**

```bash
git add internal/agent/tool/browserx/tools_test.go
git commit -m "test(browserx): add tool metadata and integration tests"
```

---

### Task 14: Final Verification

**Step 1: Full build**

Run: `go build -trimpath ./cmd/friday`
Expected: Clean build, binary produced

**Step 2: Full test suite**

Run: `GOMAXPROCS=4 go test -race ./...`
Expected: All tests pass, no race conditions

**Step 3: Verify no lint issues**

Run: `go vet ./...`
Expected: Clean

**Step 4: Review file count**

Run: `ls -la internal/agent/tool/browserx/`
Expected: ~15 files (implementation + tests)

```
cookie.go, cookie_test.go
display.go, display_test.go
manager.go, manager_test.go
result.go
selector.go, selector_test.go
stealth.go
tool_click.go, tool_close.go, tool_list.go
tool_navigate.go, tool_open.go, tool_read.go
tool_scroll.go, tool_select.go, tool_type.go
tools.go, tools_test.go
```
