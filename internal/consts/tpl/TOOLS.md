# TOOLS (LLM-Facing Guide)

This file documents the tools currently registered by the Agent and how to call them reliably.

## Quick Rules

1. Use `list` before `read` to discover the correct path.
2. Prefer `edit` for minimal changes; use `write` for full overwrite.
3. Use `exec` for short synchronous commands.
4. Use `process` for long-running background jobs.
5. Use exact parameter names when possible.
6. Paths can be relative to workspace or absolute. Out-of-scope paths fail with `path not allowed`.

## Tool Index

- `read`: read file content
- `write`: write or overwrite file content
- `edit`: minimal file updates (text or line-range replacement)
- `list`: list directory entries
- `delete`: delete a file
- `file`: legacy multi-operation file tool (compat only)
- `exec`: run short commands synchronously
- `process`: manage background processes (`start/status/log/kill/list`)
- `message`: send message to a channel/chat

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

Purpose: create or overwrite a file.

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
- `line range out of bounds`
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

Example:
```json
{"operation":"read_file","path":"README.md"}
```

---

## 7) `exec`

Purpose: run short commands synchronously.

Parameters:
- `command` (required):
- string, for example `"go test ./..."`
- or string array, for example `["/usr/local/go/bin/go","test","./..."]`
- `working_dir` (string, optional)
- `timeout` (number, optional, seconds)

Response:
- `success` (bool), true only when `exit_code == 0`
- `command` (string)
- `exit_code` (number)
- `stdout` (string)
- `stderr` (string)
- `working_dir` (string)

Notes:
- Non-zero exit is returned as `success=false` with `exit_code`; it is not treated as a tool error.
- Timeout returns an error: `command timeout after ...`.

Example:
```json
{"command":["/usr/local/go/bin/go","test","./internal/agent/tool/shellx"],"timeout":120}
```

---

## 8) `process`

Purpose: manage long-running background processes without blocking the dialog loop.

Common parameter:
- `action` (required): `start|status|log|kill|list`

### `action=start`
Parameters:
- `command` (required, string or []string)
- `working_dir` (optional)

Response:
- `success`, `process_id`, `running`, `command`, `working_dir`, `started_at`

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
- `tail` (optional, bytes, default 4096)

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
