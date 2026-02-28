# TOOLS (LLM-Facing Guide)

This file documents the tools currently registered by the Agent and how to call them reliably.

## Quick Rules

1. Use `list` before `read` to discover the correct path.
2. Prefer `edit` for minimal changes; use `write` for full overwrite.
3. Use `exec` for short synchronous commands (max 600s).
4. Use `process` for long-running background jobs (max 16 concurrent).
5. Use `cronx` to schedule recurring or one-shot tasks; prefer `schedule_type=every` for simple intervals.
6. Use `web_search` + `web_fetch` for web research; use `render_js` only when direct fetch fails on JS-heavy pages.
7. Use `http_request` for calling external APIs (REST/JSON); use `web_fetch` for reading web pages.
8. Use `agent` to delegate complex coding tasks to CLI agents (Claude Code, Codex).
9. Use `get_time` to get current date/time; never guess or hallucinate dates and weekdays.
10. Use `browser` for browser automation; always `open` first, then interact, then `close`.
11. Use `mcp` to connect to external MCP servers and call their tools.
12. Paths can be relative to workspace or absolute. Out-of-scope paths fail with `path not allowed`.

## Tool Index

| Tool | Description |
|------|-------------|
| `read` | Read file content |
| `write` | Write or overwrite file content |
| `edit` | Minimal file updates (text or line-range replacement) |
| `list` | List directory entries |
| `delete` | Delete a file |
| `file` | Legacy multi-operation file tool (compat only) |
| `exec` | Run short commands synchronously |
| `process` | Manage background processes (`start/status/log/kill/list`) |
| `message` | Send message to a channel/chat |
| `get_time` | Get current date, time, weekday, and timezone info |
| `knowledge_search` | Search local knowledge base (requires `qmd`) |
| `knowledge_get` | Retrieve full document from knowledge base (requires `qmd`) |
| `cronx` | Manage scheduled cron jobs (`create/list/delete/update`) |
| `web_fetch` | Fetch a URL and extract content as markdown |
| `web_search` | Search the web via Brave Search API (requires `BRAVE_API_KEY`) |
| `http_request` | Make HTTP requests to external APIs |
| `agent` | Delegate coding tasks to CLI agents (Claude Code, Codex) |
| `browser` | Browser automation with stealth anti-detection |
| `mcp` | Connect to external MCP servers and call their tools |

---

## 1) `read`

Purpose: read a single file.

Parameters:
- `path` (string, required)

Success response:
- `success` (bool)
- `path` (string)
- `content` (string)
- `size` (number)

Example:
```json
{"path":"internal/agent/agent.go"}
```

---

## 2) `write`

Purpose: create or overwrite a file. Creates parent directories automatically.

Parameters:
- `path` (string, required)
- `content` (string, required)

Success response:
- `success` (bool)
- `path` (string)
- `size` (number)

Example:
```json
{"path":"workspace/notes.txt","content":"hello"}
```

---

## 3) `edit`

Purpose: modify an existing file with minimal changes.

Modes (choose one):
1. Text replacement mode
- `path` (required)
- `old_text` (required)
- `new_text` (required)
- `replace_all` (optional, default `true`)

2. Line-range replacement mode
- `path` (required)
- `start_line` (required, 1-based)
- `end_line` (required, 1-based)
- `new_text` (required)

Success response:
- `success` (bool)
- `path` (string)
- `changes` (number)
- `size` (number)

Common failures:
- `old_text not found in file`
- `old_text must not be empty when provided`
- `line range out of bounds`
- `end_line must be >= start_line`
- `edit made no changes`

Example (text replacement):
```json
{"path":"a.txt","old_text":"foo","new_text":"bar","replace_all":true}
```

Example (line replacement):
```json
{"path":"a.txt","start_line":10,"end_line":12,"new_text":"new lines"}
```

---

## 4) `list`

Purpose: list directory contents.

Parameters:
- `path` (string, required)

Success response:
- `success` (bool)
- `path` (string)
- `files` (array)
- `count` (number)

`files[i]` main fields:
- `name`, `path`, `type` (`file|directory`), `size`, `mode`

Example:
```json
{"path":"internal/agent/tool"}
```

---

## 5) `delete`

Purpose: delete a single file.

Parameters:
- `path` (string, required)

Success response:
- `success` (bool)
- `path` (string)

Example:
```json
{"path":"tmp/output.log"}
```

---

## 6) `file` (legacy compatibility tool)

Purpose: historical multi-operation tool with `read_file|write_file|list_dir|delete_file`.

Recommendation: prefer `read/write/list/delete`; use `file` only for compatibility.

Parameters:
- `operation` (string, optional but strongly recommended)
- plus operation-specific parameters:
- `path` (required)
- `content` (required for `write_file`)

Notes:
- If `operation` is omitted, behavior is inferred and may be unstable.
- `list_dir` response includes `mtime` (Unix timestamp) per entry.

Example:
```json
{"operation":"read_file","path":"README.md"}
```

---

## 7) `exec`

Purpose: run short commands synchronously.

Parameters:
- `command` (required):
- string, for example `"go test ./..."` (runs via `sh -c`)
- or string array, for example `["/usr/local/go/bin/go","test","./..."]` (exec form, no shell)
- `working_dir` (string, optional)
- `timeout` (number, optional, seconds, default 60, max 600)

Response:
- `success` (bool), true only when `exit_code == 0`
- `command` (string)
- `exit_code` (number)
- `stdout` (string, max 1 MiB)
- `stderr` (string, max 1 MiB)
- `working_dir` (string)
- `truncated` (bool, present if output was truncated)

Notes:
- Non-zero exit is returned as `success=false` with `exit_code`; it is not treated as a tool error.
- Timeout returns an error: `command timeout after ...`.
- String commands use `sh -c`; array commands use exec form (no shell interpretation).

Example:
```json
{"command":["/usr/local/go/bin/go","test","./internal/agent/tool/shellx"],"timeout":120}
```

---

## 8) `process`

Purpose: manage long-running background processes without blocking the dialog loop.

Common parameter:
- `action` (required): `start|status|log|kill|list`

Limits: max 16 concurrent active processes. Finished processes are retained for 30 minutes.

### `action=start`
Parameters:
- `command` (required, string or []string)
- `working_dir` (optional)

Response:
- `success`, `process_id` (e.g. `"proc-1"`), `running`, `command`, `working_dir`, `started_at`

### `action=status`
Parameters:
- `process_id` (required)
- alias: `pid`

Response:
- `process_id`, `command`, `working_dir`, `running`, `started_at`
- when finished: `ended_at`, `exit_code`
- when abnormal: `error`

### `action=log`
Parameters:
- `process_id` (required)
- `tail` (optional, bytes, default 4096, max 1 MiB)

Response:
- status fields plus `stdout`, `stderr`, `tail`

### `action=kill`
Parameters:
- `process_id` (required)

Response:
- status fields plus `killed` (bool)

### `action=list`
Parameters:
- none

Response:
- array of process status snapshots

Examples:
```json
{"action":"start","command":"/usr/local/go/bin/go test ./...","working_dir":"."}
```

```json
{"action":"status","process_id":"proc-1"}
```

```json
{"action":"log","process_id":"proc-1","tail":8000}
```

```json
{"action":"kill","process_id":"proc-1"}
```

```json
{"action":"list"}
```

---

## 9) `message`

Purpose: send a message to a target channel/chat.

Parameters:
- `chanId` (required)
- `chatId` (required)
- `content` (required)

Accepted aliases:
- `chan_id`, `channelId`, `channel_id`
- `chat_id`

Success response:
- `success` (bool)
- `chanId` (string)
- `chatId` (string)
- `content` (string)

Common failures:
- `channel not found: ...`
- `failed to send message: ...`

Example:
```json
{"chanId":"telegram-main","chatId":"123456","content":"Task completed."}
```

---

## 10) `get_time`

Purpose: get the current date and time. Use this tool to avoid hallucinating dates, times, or weekdays.

Parameters:
- `timezone` (string, optional) - IANA timezone name (e.g. `Asia/Shanghai`, `America/New_York`, `Europe/London`). Defaults to the system local timezone if omitted.

Success response:
- `datetime` (string) - RFC3339 format (e.g. `2025-06-15T14:30:00+08:00`)
- `date` (string) - `YYYY-MM-DD` format
- `time` (string) - `HH:MM:SS` format
- `weekday` (string) - e.g. `Sunday`, `Monday`
- `unix` (number) - Unix timestamp in seconds
- `timezone` (string) - timezone abbreviation (e.g. `CST`, `EST`)
- `timezone_offset` (string) - UTC offset (e.g. `+08:00`, `-05:00`)

Common failures:
- `invalid timezone "...": ...`

Examples:
```json
{}
```

```json
{"timezone":"America/New_York"}
```

---

## 11) `cronx`

Purpose: manage scheduled cron jobs — create periodic or one-shot tasks, list existing jobs, delete or update them.

Common parameter:
- `action` (required): `create|list|delete|update`

### `action=create`
Parameters:
- `name` (string, required) - human-readable job name
- `schedule_type` (string, required) - `every` (interval like `"5m"`), `cron` (5-field cron expression), or `at` (ISO 8601 one-shot timestamp)
- `schedule` (string, required) - schedule value matching `schedule_type`
- `prompt` (string, required) - message/instruction sent to the agent when the job fires
- `session_target` (string, optional) - `main` (shared conversation) or `isolated` (dedicated session, default)
- `channel_id` (string, optional) - delivery channel for isolated jobs (defaults to current channel)
- `chat_id` (string, optional) - delivery chat for isolated jobs (defaults to current chat)
- `enabled` (bool, optional) - defaults to `true`

Response:
- `success`, `job_id`, `name`, `message`

### `action=list`
Parameters: none

Response:
- `jobs` (array with `job_id`, `name`, `agent_id`, `schedule_type`, `schedule`, `session_target`, `enabled`, `created_at`, `last_run_at`, `next_run_at`, `prompt` (truncated at 120 chars))
- `count` (number)

### `action=delete`
Parameters:
- `job_id` (string, required)

Response:
- `success`, `job_id`, `message`

Notes:
- The built-in heartbeat job cannot be deleted.

### `action=update`
Parameters:
- `job_id` (string, required)
- any of: `name`, `schedule_type`, `schedule`, `prompt`, `enabled`, `session_target`, `channel_id`, `chat_id`

Response:
- `success`, `job_id`, `message`

Notes:
- The built-in heartbeat job cannot be updated.
- At least one field must be changed; `no fields to update` error otherwise.

Examples:
```json
{"action":"create","name":"daily digest","schedule_type":"cron","schedule":"0 9 * * *","prompt":"Summarize yesterday's activity"}
```

```json
{"action":"create","name":"remind me","schedule_type":"at","schedule":"2025-06-01T14:00:00Z","prompt":"Time for the meeting!"}
```

```json
{"action":"create","name":"health check","schedule_type":"every","schedule":"5m","prompt":"Check service status"}
```

```json
{"action":"list"}
```

```json
{"action":"delete","job_id":"cronx-daily-digest-1717200000"}
```

```json
{"action":"update","job_id":"cronx-health-check-1717200000","enabled":false}
```

---

## 12) `knowledge_search`

Purpose: search the local knowledge base (markdown docs, notes, meeting transcripts) using hybrid BM25 + vector semantic search. Returns relevant snippets instead of full documents to save tokens.

Availability: only registered when `qmd` CLI is installed on the host.

Parameters:
- `query` (string, required) - search keywords or natural-language question
- `collection` (string, optional) - restrict search to a named collection
- `mode` (string, optional) - `query` (default, hybrid+rerank), `search` (BM25 only), `vsearch` (vector only)
- `limit` (number, optional) - max results, default 5

Success response:
- `success` (bool)
- `query` (string)
- `mode` (string)
- `count` (number)
- `results` (array of result objects)

Common failures:
- `query is required`
- `mode must be one of: query, search, vsearch`
- `qmd query failed: ...`

Example:
```json
{"query":"how to deploy the service","mode":"query","limit":3}
```

---

## 13) `knowledge_get`

Purpose: retrieve a specific document from the local knowledge base by file path or document ID. Use `knowledge_search` first to find relevant documents, then use this tool only when you need the full content.

Availability: only registered when `qmd` CLI is installed on the host.

Parameters:
- `path` (string, required) - file path or document ID (e.g. `#abc123`)

Success response:
- `success` (bool)
- `path` (string)
- `content` (string)
- `size` (number)

Common failures:
- `path is required`
- `qmd get failed: ...`

Example:
```json
{"path":"docs/deployment.md"}
```

```json
{"path":"#abc123"}
```

---

## 14) `web_fetch`

Purpose: fetch a URL and extract its main content as clean markdown. Handles HTML pages (via readability extraction), JSON endpoints, and optionally JS-heavy pages via Cloudflare Browser Rendering.

Parameters:
- `url` (string, required) - the URL to fetch (must be http or https)
- `max_chars` (number, optional) - maximum characters to return, default 50000
- `render_js` (bool, optional) - set `true` to use Cloudflare Browser Rendering for JS-heavy SPAs (requires `CLOUDFLARE_API_TOKEN` and `CLOUDFLARE_ACCOUNT_ID` env vars; falls back to direct fetch if not configured)

Content handling:
- `text/markdown` response → used directly (content negotiation with Cloudflare-fronted sites)
- `text/html` response → readability extraction → markdown conversion
- `application/json` response → pretty-printed JSON
- other → raw text

Security:
- Only `http` and `https` schemes allowed
- Private/loopback/link-local addresses are blocked (SSRF protection)
- Redirects capped at 5; redirects to private addresses are blocked
- Response body capped at 5 MiB

Success response (JSON string):
- `url` (string) - final URL after redirects
- `title` (string) - page title (HTML only)
- `status` (number) - HTTP status code
- `length` (number) - content length in characters
- `truncated` (bool) - whether content was truncated to `max_chars`
- `content` (string) - extracted content

Common failures:
- `url is required`
- `only http and https URLs are allowed`
- `access to private/internal addresses is not allowed`
- `cloudflare render: ...` (when `render_js=true`)

Examples:
```json
{"url":"https://example.com/blog/post-1"}
```

```json
{"url":"https://api.example.com/data","max_chars":10000}
```

```json
{"url":"https://spa-heavy-site.com","render_js":true}
```

---

## 15) `web_search`

Purpose: search the web using Brave Search API. Returns titles, URLs, and snippets for the top results.

Availability: always registered, but requires `BRAVE_API_KEY` environment variable at execution time. Returns an error if the env var is not set.

Parameters:
- `query` (string, required) - the search query
- `count` (number, optional) - number of results to return, 1–10, default 5

Success response (formatted text):
```
Results for: <query>

1. <title>
   <url>
   <snippet>

2. ...
```

Common failures:
- `query is required`
- `BRAVE_API_KEY environment variable is not set; web search is unavailable`
- `search failed: ...`

Examples:
```json
{"query":"Go readability library","count":3}
```

```json
{"query":"cloudflare browser rendering markdown API"}
```

---

## 16) `http_request`

Purpose: make an HTTP request to an external API. Use this for REST/JSON API calls; use `web_fetch` for reading web pages.

Parameters:
- `url` (string, required) - target URL (http or https)
- `method` (string, required) - `GET`, `POST`, `PUT`, `PATCH`, or `DELETE`
- `headers` (object, optional) - custom request headers as key-value pairs
- `body` (string, optional) - request body (typically JSON string)
- `timeout` (number, optional) - timeout in seconds, default 30, max 120

Notes:
- If `body` is provided and no `Content-Type` header is set, defaults to `application/json`.

Security:
- Only `http` and `https` schemes allowed
- Private/loopback/link-local addresses are blocked (SSRF protection)
- Redirects capped at 5; redirects to private addresses are blocked
- Response body capped at 5 MiB

Success response (JSON string):
- `status` (number) - HTTP response status code
- `headers` (object) - response headers
- `body` (string) - response body (max 50000 chars)
- `length` (number) - length of body returned
- `truncated` (bool) - whether body was truncated

Common failures:
- `url is required`
- `only http and https URLs are allowed`
- `access to private/internal addresses is not allowed`
- `unsupported method "..."; allowed: GET, POST, PUT, PATCH, DELETE`
- `request failed: ...`

Examples:
```json
{"method":"GET","url":"https://api.example.com/users/1"}
```

```json
{"method":"POST","url":"https://api.example.com/users","headers":{"Authorization":"Bearer token123"},"body":"{\"name\":\"Alice\"}"}
```

```json
{"method":"DELETE","url":"https://api.example.com/users/1","timeout":10}
```

---

## 17) `agent`

Purpose: delegate complex coding tasks to CLI agents (Claude Code or Codex). Supports creating sessions, sending follow-up messages, checking status, and managing session lifecycle.

Common parameter:
- `action` (required): `create|send|status|list|destroy`

Limits: max 8 concurrent sessions. Sync execution timeout is 600 seconds.

### `action=create`
Start a new agent session.

Parameters:
- `backend` (string, required) - `claude-code` or `codex`
- `prompt` (string, required) - task/instruction for the agent
- `working_dir` (string, optional) - working directory (defaults to agent workspace)
- `system_prompt` (string, optional) - additional system prompt
- `max_turns` (number, optional) - maximum agentic turns (Claude Code only)
- `async` (bool, optional, default false) - run in background

Response (sync):
- `session_id` (e.g. `"as-1"`), `backend`, `cli_session_id`, `status` (`"completed"`), `result`

Response (async):
- `session_id`, `backend`, `status` (`"running"`)

### `action=send`
Send a follow-up message to an existing session.

Parameters:
- `session_id` (string, required)
- `prompt` (string, required) - follow-up message
- `async` (bool, optional, default false)

Response (sync):
- `session_id`, `cli_session_id`, `status`, `result`

Response (async):
- `session_id`, `status` (`"running"`)

### `action=status`
Check execution status of a session.

Parameters:
- `session_id` (string, required)

Response:
- `session_id`, `backend`, `status` (`running|completed|failed`), `created_at`
- when completed: `cli_session_id`, `result`

### `action=list`
List all sessions.

Parameters: none

Response:
- `sessions` (array with `session_id`, `backend`, `status`, `created_at`)

### `action=destroy`
Terminate and remove a session.

Parameters:
- `session_id` (string, required)

Response:
- `success` (bool), `session_id`

Common failures:
- `backend is required for create action`
- `prompt is required for create action`
- `unknown backend: ... (available: claude-code, codex)`
- `<backend> CLI not found in PATH`
- `working_dir "..." is outside agent workspace "..."`
- `max sessions (8) reached, destroy one first`
- `session ... not found`
- `session ... has no CLI session ID (was it completed?)`

Examples:
```json
{"action":"create","backend":"claude-code","prompt":"Write unit tests for internal/config/validate.go"}
```

```json
{"action":"create","backend":"codex","prompt":"Refactor the error handling in cmd/serve.go","async":true}
```

```json
{"action":"send","session_id":"as-1","prompt":"Also add a benchmark test"}
```

```json
{"action":"status","session_id":"as-1"}
```

```json
{"action":"list"}
```

```json
{"action":"destroy","session_id":"as-1"}
```

---

## 18) `browser`

Purpose: browser automation with stealth anti-detection. Supports navigation, element interaction, screenshots, content extraction, and JavaScript evaluation. Powered by go-rod with stealth mode enabled by default.

Common parameter:
- `operation` (required): `open|close|navigate|click|type|screenshot|extract|evaluate|wait|scroll|list_sessions`

### `operation=open`
Open a new browser session.

Parameters:
- `headless` (bool, optional, default `true`) - run browser in headless mode

Response:
- `session_id` (string) - use this ID for all subsequent operations
- `headless` (bool)
- `stealth` (bool) - always `true`

### `operation=close`
Close a browser session and release resources.

Parameters:
- `session_id` (string, required)

Response:
- `success` (bool)

### `operation=navigate`
Navigate to a URL.

Parameters:
- `session_id` (string, required)
- `url` (string, required) - URL to navigate to
- `wait_load` (bool, optional, default `true`) - wait for page load after navigation

Response:
- `url` (string) - final URL
- `title` (string) - page title

### `operation=click`
Click an element on the page.

Parameters:
- `session_id` (string, required)
- `selector` (string, required) - CSS or XPath selector
- `selector_type` (string, optional) - `"css"` (default) or `"xpath"`

Response:
- `success` (bool)
- `selector` (string)

### `operation=type`
Type text into an input element.

Parameters:
- `session_id` (string, required)
- `selector` (string, required) - CSS or XPath selector for the input element
- `text` (string, required) - text to input
- `selector_type` (string, optional) - `"css"` (default) or `"xpath"`
- `clear` (bool, optional) - clear existing text before typing

Response:
- `success` (bool)

### `operation=screenshot`
Take a screenshot of the page or a specific element.

Parameters:
- `session_id` (string, required)
- `selector` (string, optional) - CSS/XPath selector for element screenshot; omit for full page
- `selector_type` (string, optional) - `"css"` (default) or `"xpath"`
- `format` (string, optional) - `"png"` (default) or `"jpeg"`

Response:
- `data` (string) - base64-encoded screenshot image
- `format` (string) - `"png"` or `"jpeg"`

### `operation=extract`
Extract text, HTML, or attributes from elements.

Parameters:
- `session_id` (string, required)
- `selector` (string, required) - CSS or XPath selector
- `selector_type` (string, optional) - `"css"` (default) or `"xpath"`
- `attribute` (string, optional) - HTML attribute to extract (e.g. `href`, `src`)
- `all` (bool, optional) - extract all matching elements instead of first

Response (single element):
- `text` (string) - text content
- `html` (string) - outer HTML
- `attribute` (string) - attribute value (if `attribute` was specified)

Response (all elements, `all=true`):
- `elements` (array of `{text, html, attribute}`)
- `count` (number)

### `operation=evaluate`
Execute JavaScript code on the page.

Parameters:
- `session_id` (string, required)
- `script` (string, required) - JavaScript code to execute

Response:
- `result` (any) - return value from the script

Notes:
- If the script is not wrapped in a function, it is auto-wrapped as `() => { <script> }`.

### `operation=wait`
Wait for an element to appear and become visible.

Parameters:
- `session_id` (string, required)
- `selector` (string, required) - CSS or XPath selector
- `selector_type` (string, optional) - `"css"` (default) or `"xpath"`
- `timeout` (number, optional) - timeout in seconds, default 30

Response:
- `success` (bool)

Common failures:
- `element not found: selector '...' timed out after 30s`
- `element '...' not visible after 30s`

### `operation=scroll`
Scroll the page or scroll an element into view.

Parameters:
- `session_id` (string, required)
- `selector` (string, optional) - if provided, scrolls the element into view
- `selector_type` (string, optional) - `"css"` (default) or `"xpath"`
- `x` (number, optional) - horizontal scroll offset in pixels (when no selector)
- `y` (number, optional) - vertical scroll offset in pixels (when no selector)

Response:
- `success` (bool)

### `operation=list_sessions`
List all active browser sessions.

Parameters: none

Response:
- `sessions` (array)
- `count` (number)

Common failures (general):
- `session_id is required`
- `session not found: ...`
- `unknown operation "..."`

Examples:
```json
{"operation":"open","headless":true}
```

```json
{"operation":"navigate","session_id":"bs-1","url":"https://example.com"}
```

```json
{"operation":"click","session_id":"bs-1","selector":"button.submit"}
```

```json
{"operation":"type","session_id":"bs-1","selector":"#search-input","text":"hello world","clear":true}
```

```json
{"operation":"screenshot","session_id":"bs-1","format":"png"}
```

```json
{"operation":"extract","session_id":"bs-1","selector":"h1","all":false}
```

```json
{"operation":"extract","session_id":"bs-1","selector":"a.link","attribute":"href","all":true}
```

```json
{"operation":"evaluate","session_id":"bs-1","script":"return document.title"}
```

```json
{"operation":"wait","session_id":"bs-1","selector":"#result","timeout":10}
```

```json
{"operation":"scroll","session_id":"bs-1","y":500}
```

```json
{"operation":"scroll","session_id":"bs-1","selector":"#footer"}
```

```json
{"operation":"list_sessions"}
```

```json
{"operation":"close","session_id":"bs-1"}
```

---

## 19) `mcp`

Purpose: connect to external MCP (Model Context Protocol) servers and call their tools. Supports stdio and HTTP transports. Use `list_servers`/`list_tools` to discover available tools, then `call_tool` to invoke them.

Common parameter:
- `action` (required): `connect|disconnect|list_servers|list_tools|call_tool`

### `action=connect`
Add and connect to an MCP server.

Parameters:
- `name` (string, required) - server name
- `transport` (string, required) - `"stdio"` or `"http"`
- `command` (string, required for stdio) - command to run
- `args` (array, optional for stdio) - command arguments
- `env` (object, optional for stdio) - environment variables
- `url` (string, required for http) - server URL

Response:
- `status` (`"connected"`)
- `server` (string)

### `action=disconnect`
Disconnect from an MCP server.

Parameters:
- `server` (string, required) - server name

Response:
- `status` (`"disconnected"`)
- `server` (string)

### `action=list_servers`
List all connected MCP servers.

Parameters: none

Response:
- `servers` (array of `{name, transport, status, error?}`)

### `action=list_tools`
List available tools from MCP servers.

Parameters:
- `server` (string, optional) - filter by server name; if omitted, lists tools from all servers

Response:
- `tools` (array of `{server, name, description?}`)

### `action=call_tool`
Invoke a tool on an MCP server.

Parameters:
- `server` (string, required) - server name
- `tool` (string, required) - tool name
- `arguments` (object, optional) - tool arguments as JSON

Response:
- `server` (string)
- `tool` (string)
- `content` (array of `{type, text}` or `{type, mime_type, data}`)
- `is_error` (bool, present if the tool returned an error)

Common failures:
- `action is required`
- `name is required for connect action`
- `transport is required for connect action`
- `command is required for stdio transport`
- `url is required for http transport`
- `server "..." not found`
- `tool is required for call_tool action`
- `call tool .../...: ...`

Examples:
```json
{"action":"connect","name":"my-server","transport":"stdio","command":"npx","args":["@my/mcp-server"]}
```

```json
{"action":"connect","name":"remote","transport":"http","url":"http://localhost:8080/mcp"}
```

```json
{"action":"list_servers"}
```

```json
{"action":"list_tools","server":"my-server"}
```

```json
{"action":"call_tool","server":"my-server","tool":"search","arguments":{"query":"hello"}}
```

```json
{"action":"disconnect","server":"my-server"}
```

---

## Suggested LLM Playbooks

### Code change task
1. `list` to locate files
2. `read` target files
3. `edit` for minimal changes (or `write` when needed)
4. `exec` for tests/build/validation
5. `message` to send result summary when needed

### Long-running task
1. `process start`
2. poll with `process status` and `process log`
3. `process kill` if cancellation is needed

### Complex coding task (delegation)
1. `agent create` with appropriate backend and prompt
2. For async: poll with `agent status`
3. `agent send` for follow-up instructions
4. `agent destroy` when done

### Knowledge-assisted task
1. `knowledge_search` to find relevant docs/notes
2. `knowledge_get` only when you need the full content of a specific result
3. Incorporate retrieved knowledge into your response or code changes

### Scheduling a recurring task
1. `cronx` with `action=create` to set up the job
2. `cronx` with `action=list` to verify it was registered
3. `cronx` with `action=update` to adjust schedule or disable later

### Web research task
1. `web_search` to find relevant pages
2. `web_fetch` to read the most relevant URLs from search results
3. Use `render_js=true` only for JS-heavy SPAs where direct fetch returns empty/broken content
4. Summarize findings and incorporate into your response

### External API interaction
1. `http_request` with appropriate method and headers
2. Parse the JSON response body
3. Use results in your workflow

### Browser automation task
1. `browser open` to start a session (headless by default)
2. `browser navigate` to the target URL
3. `browser wait` for key elements to appear
4. `browser click`/`browser type` to interact
5. `browser extract` or `browser screenshot` to capture results
6. `browser close` to release resources

### MCP server integration
1. `mcp connect` to add an external MCP server (stdio or http)
2. `mcp list_tools` to discover available tools
3. `mcp call_tool` to invoke tools with arguments
4. `mcp disconnect` when done

### Date/time-aware task
1. `get_time` to get the current date, time, and weekday
2. Use the result to avoid hallucinating temporal information
