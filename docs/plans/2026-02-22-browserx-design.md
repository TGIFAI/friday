# Browserx — Stealth Browser Tool for Friday

**Goal:** Add a headful browser tool family (`browserx`) to friday, powered by go-rod + go-rod/stealth, enabling LLM-driven browser automation with human-in-the-loop interaction for login/CAPTCHA scenarios.

**Decision Date:** 2026-02-22

---

## Context

friday's existing `webx` tools (`web_fetch`, `web_search`) use HTTP direct fetch + Cloudflare Browser Rendering. This works for static pages and simple JS rendering, but fails for:

- Pages behind authentication (human must log in)
- Heavy SPA pages that require full browser execution
- Sites with aggressive bot detection (Cloudflare, DataDome, Akamai)

A local headless/headful browser with anti-detection is needed to fill this gap.

## Technology Choice: go-rod + go-rod/stealth

Evaluated three approaches:

| | chromedp + chromedp-undetected | **go-rod + go-rod/stealth** | chromedp + stealth JS hybrid |
|---|---|---|---|
| Anti-detection | Partial | Full puppeteer-stealth JS set | Partial |
| API ergonomics | Verbose | Clean | Awkward |
| Maintenance | Moderate | Active | Fragile |

**Chosen: go-rod + go-rod/stealth** — most complete anti-detection (15+ evasion modules from puppeteer-extra-plugin-stealth), cleanest API, actively maintained.

### Stealth Evasions (Always On)

- `navigator.webdriver` → false
- `window.chrome`, `chrome.runtime`, `chrome.app`, `chrome.csi`, `chrome.loadTimes` mocked
- `navigator.plugins` realistic list
- WebGL vendor/renderer spoofed
- Canvas fingerprint normalized
- `iframe.contentWindow` cross-origin fix
- Media codecs mimicked
- `--disable-blink-features=AutomationControlled` Chrome flag
- `--headless=new` in headless mode (near-perfect fingerprint match)

## Architecture

### Package Structure

```
internal/agent/tool/browserx/
├── manager.go           BrowserManager: lifecycle, multi-user sessions, resource limits
├── cookie.go            Per-user cookie persistence (JSON serialize/deserialize)
├── display.go           Desktop environment detection
├── stealth.go           Rod + stealth configuration wrapper
├── result.go            BrowserResult unified return struct
├── selector.go          Multi-mode selector resolution (CSS/text/xpath/coord)
├── tool_open.go         browser_open
├── tool_read.go         browser_read
├── tool_navigate.go     browser_navigate
├── tool_click.go        browser_click
├── tool_type.go         browser_type
├── tool_select.go       browser_select
├── tool_scroll.go       browser_scroll
├── tool_close.go        browser_close
├── tool_list.go         browser_list
└── *_test.go
```

### Multi-User Isolation

Single Chrome process, multiple incognito contexts, per-user cookie persistence:

```
BrowserManager (singleton, lazy init)
│
├── rod.Browser (single process, stealth flags)
│
├── sessions map[userKey]*UserSession
│   ├── "telegram:12345" → UserSession
│   │   ├── *rod.Browser (incognito context)
│   │   ├── pages map[pageID]*PageInfo
│   │   └── cookieFile: ~/.friday/browser-data/telegram_12345/cookies.json
│   └── "lark:67890" → UserSession
│       └── ...
│
└── cleanup goroutine (idle timeout eviction)
```

**User key derivation:** `channelID:chatID` from `consts.CtxKeyChannelID` + `consts.CtxKeyChatID` (same as cli-provider).

**Why incognito contexts (not multiple Chrome processes):**
- One Chrome ~200-300MB RAM; N processes doesn't scale
- `browser.Incognito()` gives full cookie/cache/storage isolation
- Cookie persistence handled manually via JSON serialization

### Cookie Lifecycle

```
Session created  → loadCookies(file) → context.SetCookies(...)
After navigate   → page.Cookies() → saveCookies(file)
Session idle     → final saveCookies → context.Close()
Friday restart   → sessions rebuilt, loadCookies restores login state
```

Storage path: `~/.friday/browser-data/<userKey>/cookies.json`

### Desktop Detection

| Platform | Detection | Notes |
|----------|-----------|-------|
| Linux | `$DISPLAY` or `$WAYLAND_DISPLAY` non-empty | Covers X11 + Wayland |
| macOS | Check if `WindowServer` process exists | Absent in pure SSH remote |
| Windows | Default true | Headless servers rare |

Behavior:
- `headless=false` (default) + no display → return error: "No desktop environment detected"
- `headless=true` → works regardless of display
- `headless=false` + display → open headful Chrome window

## Tool Definitions

### Unified Response

```go
type BrowserResult struct {
    PageID     string `json:"page_id"`
    URL        string `json:"url"`
    Title      string `json:"title"`
    Screenshot string `json:"screenshot_base64"` // JPEG base64
    Content    string `json:"content,omitempty"` // markdown
    Success    bool   `json:"success"`
    Message    string `json:"message,omitempty"`
}
```

- Navigation tools (open/read/navigate) return screenshot + content
- Interaction tools (click/type/select/scroll) return screenshot only
- LLM calls `browser_read` when it needs content after interaction

### Screenshot Config

- Format: JPEG, quality=80
- Viewport: 1280x800
- Viewport-only (not full page)

### Tools

| Tool | Parameters | Returns | Wait Strategy |
|------|-----------|---------|---------------|
| `browser_open` | url, headless? | screenshot + content | WaitLoad + WaitStable |
| `browser_read` | page_id? | screenshot + content | none (instant) |
| `browser_navigate` | url, page_id? | screenshot + content | WaitLoad + WaitStable |
| `browser_click` | selector, page_id? | screenshot | WaitStable |
| `browser_type` | selector, text, clear?, page_id? | screenshot | none |
| `browser_select` | selector, value, page_id? | screenshot | WaitStable |
| `browser_scroll` | direction(up/down), page_id? | screenshot | WaitStable (lazy load) |
| `browser_close` | page_id? | confirmation | n/a |
| `browser_list` | none | page list (no screenshot) | n/a |

All wait strategies have a 10s global timeout fallback.

### Selector Resolution

All interaction tools accept a `selector` string with prefix dispatch:

| Prefix | Example | Resolution |
|--------|---------|------------|
| (none) | `button#login` | CSS selector |
| `text=` | `text=Sign In` | Rod regex text match `page.ElementR("*", text)` |
| `xpath=` | `xpath=//button[contains(text(),'OK')]` | XPath |
| `coord=` | `coord=350,420` | Pixel coordinates via `page.ElementFromPoint(x,y)` |

Text-based selectors are recommended for LLM usage (most natural).

## Human-in-the-Loop Flow

```
User(Telegram): "Help me check my private repos on GitHub"

→ LLM: browser_open(url="https://github.com")
  → loadCookies → navigate → detect state
  → return: {screenshot: login page, content: "Sign in to GitHub..."}
→ LLM sees login page:
  "I've opened GitHub in the browser window. Please log in, then let me know."

User: (interacts with headful Chrome window — types credentials, passes 2FA)
User(Telegram): "Done"

→ LLM: browser_read()
  → return: {screenshot: dashboard, content: "Dashboard..."}
  → saveCookies (login cookies now persisted)
→ LLM confirms logged in

→ LLM: browser_navigate(url="https://github.com/dave?tab=repositories&type=private")
  → return: {screenshot, content: "## Repositories\n- repo1\n- repo2..."}
→ LLM: "Your private repos: repo1, repo2..."

→ LLM: browser_close()
```

Next time GitHub cookies are still valid — no human intervention needed.

## Security

| Risk | Mitigation |
|------|-----------|
| Navigate to internal network | Reuse `utils.IsPrivateHost()` SSRF check on open/navigate |
| Resource exhaustion | Max 5 pages/user, max 20 pages global |
| Zombie pages | Idle 15min → auto close + saveCookies |
| Zombie sessions | No active pages for 30min → close context |
| Chrome crash | BrowserManager detects, auto-restart, recover sessions from cookies |
| Password in tool args | `browser_type` response does not echo input text |

## Resource Lifecycle

```
friday start → BrowserManager created (no Chrome yet, lazy)

First browser_open → detect display → launch Chrome (stealth) → create UserSession → page

Subsequent calls → reuse browser + session + page

Page idle 15min → saveCookies → page.Close()
Session idle 30min (no pages) → saveCookies → context.Close()
All sessions closed → browser.Close() → Chrome exits

friday exit → BrowserManager.Close(): save all cookies → close everything
```

## Dialog Handling

Auto-accept `alert`/`confirm`/`prompt` dialogs to prevent blocking:

```go
go page.EachEvent(func(e *proto.PageJavascriptDialogOpening) {
    _ = proto.PageHandleJavaScriptDialog{Accept: true}.Call(page)
})
```

## Relationship with webx

- `webx` unchanged: HTTP direct fetch + Cloudflare rendering + Brave search
- New `webx/fetch_rod.go`: headless rod + stealth as `render_js` fallback (when Cloudflare not configured)
- `browserx`: fully independent tool family for interactive browser sessions
- Shared: readability + html-to-markdown pipeline (extracted as common utility or browserx imports webx's exported function)

## Dependencies

- `github.com/go-rod/rod` — Browser automation
- `github.com/go-rod/stealth` — Anti-detection evasion scripts
