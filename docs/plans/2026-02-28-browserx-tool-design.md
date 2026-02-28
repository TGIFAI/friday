# browserx Tool Design

## Overview

A browser automation tool for the friday agent framework, based on go-rod with stealth anti-detection capabilities. Supports both headless and headed modes, with environment detection for headed mode validation.

## Requirements

- Single tool with `operation` parameter dispatch (like agentx/cronx)
- Based on `go-rod/rod` + `go-rod/stealth` with incremental anti-detection (Approach C)
- Support headless and headed modes; detect environment, error if headed not supported
- Remove all automation-detectable fingerprints; appear as a normal browser
- Browser sessions persist long-term (across agent loops), exit with program
- No proxy support initially

## Directory Structure

```
internal/agent/tool/browserx/
├── browser.go       # BrowserTool: implements tool.Tool interface
├── manager.go       # BrowserManager: session lifecycle (multi-instance + shutdown)
├── stealth.go       # Anti-detection: launcher flags + extra JS injection
├── env.go           # Environment detection: headed mode support check
└── ops.go           # Operation implementations (navigate/click/type/screenshot/extract/evaluate etc.)
```

## Tool Interface

- `Name()` → `"browser"`
- `ToolInfo()` → `schema.NewParamsOneOfByParams` style (consistent with webx/cronx)
- `Execute()` → dispatches by `operation` parameter
- Registration in `agent.go` `Init()`: `ag.tools.Register(browserx.NewBrowserTool())`

## Operations

| Operation | Key Params | Returns | Description |
|-----------|-----------|---------|-------------|
| `open` | `headless` (bool, default true) | `{session_id, headless, stealth}` | Create browser instance |
| `close` | `session_id` | `{success}` | Close specified instance |
| `navigate` | `session_id`, `url`, `wait_load` (bool, default true) | `{url, title, status}` | Navigate to URL |
| `click` | `session_id`, `selector`, `selector_type` (css/xpath) | `{success, selector}` | Click element |
| `type` | `session_id`, `selector`, `text`, `clear` (bool) | `{success}` | Input text |
| `screenshot` | `session_id`, `selector` (optional), `format` (png/jpeg) | `{data}` (base64) | Capture screenshot |
| `extract` | `session_id`, `selector`, `attribute` (optional), `all` (bool) | `{content}` or `{elements: [...]}` | Extract element content/attributes |
| `evaluate` | `session_id`, `script` | `{result}` | Execute arbitrary JS |
| `wait` | `session_id`, `selector`, `timeout` (seconds, default 30) | `{success}` | Wait for element |
| `scroll` | `session_id`, `selector` (optional), `x`, `y` | `{success}` | Scroll page/to element |
| `list_sessions` | none | `{sessions: [{id, headless, created_at, current_url}]}` | List active sessions |

## Session Management (BrowserManager)

```go
type Session struct {
    ID        string
    Browser   *rod.Browser
    Page      *rod.Page       // current active page
    Headless  bool
    CreatedAt time.Time
    mu        sync.Mutex      // protect Page concurrent access
}

type BrowserManager struct {
    sessions map[string]*Session
    mu       sync.RWMutex
}
```

### Lifecycle

1. `open` → create Session, launch browser, store in sessions map
2. Subsequent operations look up Session by session_id
3. `close` → gracefully close browser process, remove from map
4. **Program exit**: BrowserManager provides `Shutdown()` method that iterates all sessions and calls `Browser.Close()`. Called from `agent.go` `Close()` or via `signal.NotifyContext` shutdown hook in main

### Key Points

- Sessions persist across agent loops (not tied to loop lifecycle)
- Each session maintains one Page (current active page); navigate reuses same Page
- BrowserManager is a global singleton, shared across agent loops
- Session IDs use short UUID (8 chars) for easy LLM passing

## Anti-Detection Strategy (Layered Defense)

### Layer 1: Chrome Launch Flags (launcher configuration)

```go
func newStealthLauncher(headless bool) *launcher.Launcher {
    l := launcher.New()
    if headless {
        l.Headless(true)
        l.Set("headless", "new")  // new headless mode (Chrome 112+)
    } else {
        l.Headless(false)
    }
    l.Set("disable-blink-features", "AutomationControlled")
    l.Set("disable-features", "TranslateUI")
    l.Set("disable-infobars")
    l.Set("disable-dev-shm-usage")
    l.Set("no-first-run")
    l.Set("no-default-browser-check")
    l.Set("window-size", "1920,1080")
    l.Set("lang", "en-US")
    return l
}
```

### Layer 2: go-rod/stealth JS Injection

Use `stealth.MustPage(browser)` to create pages, automatically injecting anti-detection JS covering:
- `navigator.webdriver` → false
- `window.chrome` object simulation
- Permission API return value correction
- Plugin/MimeType array simulation
- Languages configuration
- WebGL vendor/renderer consistency
- Chrome CSI/loadTimes simulation

### Layer 3: Incremental JS Supplements

Via `Page.EvalOnNewDocument` on top of stealth base:
- Fix `navigator.permissions.query` for notifications (newer detection point)
- Shield CDP Runtime domain detection
- Simulate `navigator.connection` (NetworkInformation API)
- Fix `window.outerWidth/outerHeight` (may be 0 in headless)

### Layer 4: Behavioral

- Set realistic device profile via `browser.DefaultDevice(devices.LaptopWithMDPIScreen)`
- Realistic viewport dimensions

## Environment Detection (env.go)

```go
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

Usage in `open` operation:
```go
if !headless {
    if ok, reason := canRunHeaded(); !ok {
        return nil, fmt.Errorf("headed mode not supported: %s. Use headless=true instead", reason)
    }
}
```

## Error Handling

Following project conventions: tool errors serialize as `"ERROR: ..."` strings visible to the LLM.

Special error scenarios:
- **Session not found** → `"session 'xxx' not found. Use operation=open to create one, or operation=list_sessions to see active sessions"`
- **Environment doesn't support headed** → `"headed mode not supported: <reason>. Use headless=true instead"`
- **Element not found** → `"element not found: selector '<selector>' timed out after 30s"`
- **Browser process exited unexpectedly** → auto-remove from manager, return `"browser process exited unexpectedly, session removed"`

## Dependencies

New dependencies to add:
- `github.com/go-rod/rod` — browser automation
- `github.com/go-rod/stealth` — anti-detection JS injection

## ToolInfo Schema

Uses `schema.NewParamsOneOfByParams` with typed parameters:

```go
"operation":     { Type: schema.String,  Required: true, Desc: "..." }
"session_id":    { Type: schema.String,  Desc: "..." }
"url":           { Type: schema.String,  Desc: "..." }
"selector":      { Type: schema.String,  Desc: "..." }
"selector_type": { Type: schema.String,  Desc: "..." }
"text":          { Type: schema.String,  Desc: "..." }
"script":        { Type: schema.String,  Desc: "..." }
"headless":      { Type: schema.Boolean, Desc: "..." }
"format":        { Type: schema.String,  Desc: "..." }
"clear":         { Type: schema.Boolean, Desc: "..." }
"attribute":     { Type: schema.String,  Desc: "..." }
"all":           { Type: schema.Boolean, Desc: "..." }
"timeout":       { Type: schema.Integer, Desc: "..." }
"x":             { Type: schema.Integer, Desc: "..." }
"y":             { Type: schema.Integer, Desc: "..." }
"wait_load":     { Type: schema.Boolean, Desc: "..." }
```
