# Agent Tool Design

**Date**: 2026-02-20
**Status**: Approved

## Problem

friday needs to delegate complex coding tasks (bug fixes, feature implementation, code review, analysis) to specialized CLI agents like Claude Code and Codex. These CLI agents have their own tool systems (file editing, bash execution, etc.) and can autonomously complete multi-step development tasks.

## Decision

Implement a new Go **tool** (`agent`) in friday that wraps Claude Code and Codex CLIs via their **pipe/non-interactive modes** (`claude -p` / `codex exec`). No tmux. No config changes.

### Why Tool (not Provider, Skill, or tmux)

- **Not Provider**: Provider replaces the LLM brain. We want friday's LLM to remain the orchestrator and selectively delegate tasks. Provider mode can be added later.
- **Not Skill**: Skills are prompt-injected Markdown. LLM must manually compose shell commands each time — unreliable for multi-step tmux orchestration.
- **Not tmux**: Both CLIs have first-class non-interactive modes with structured JSON output and session resume. tmux adds unnecessary complexity (TUI parsing, ANSI codes, timing issues). Can be added later if persistent TUI observation is needed.

## Tool API

Tool name: `agent`

### Actions

#### create

Start a new CLI agent session.

```json
{
  "action": "create",
  "backend": "claude-code",
  "prompt": "Fix the auth bug in internal/auth/handler.go",
  "working_dir": "/path/to/repo",
  "system_prompt": "Focus on security best practices",
  "max_turns": 25,
  "async": false
}
```

Response:
```json
{
  "session_id": "as-1",
  "backend": "claude-code",
  "cli_session_id": "abc-123",
  "status": "completed",
  "result": "Fixed the auth bug by..."
}
```

When `async: true`, returns immediately with `"status": "running"`.

#### send

Send a follow-up message to an existing session (multi-turn).

```json
{
  "action": "send",
  "session_id": "as-1",
  "prompt": "Now fix the related tests",
  "async": false
}
```

Uses `--resume <cli_session_id>` under the hood.

#### status

Check status of an async session.

```json
{
  "action": "status",
  "session_id": "as-1"
}
```

#### list

List all active sessions.

```json
{
  "action": "list"
}
```

#### destroy

Terminate and clean up a session.

```json
{
  "action": "destroy",
  "session_id": "as-1"
}
```

## Internal Architecture

### Code Organization

```
internal/agent/tool/agentx/
├── agent.go          // Tool interface (Name, Description, ToolInfo, Execute)
├── session.go        // Session manager (create, store, lookup, cleanup)
├── backend.go        // Backend interface
├── claude_code.go    // Claude Code CLI backend
├── codex.go          // Codex CLI backend
```

### Backend Abstraction

```go
type Backend interface {
    Name() string
    Run(ctx context.Context, req *RunRequest) (*RunResult, error)
    Start(ctx context.Context, req *RunRequest) (*Process, error)
}

type RunRequest struct {
    Prompt       string
    WorkingDir   string
    SystemPrompt string
    MaxTurns     int
    ResumeID     string
}

type RunResult struct {
    CLISessionID string
    Output       string
    ExitCode     int
}
```

### CLI Discovery

No config. Use `exec.LookPath("claude")` and `exec.LookPath("codex")` at tool init time. If neither is found, the tool still registers but returns a clear error on Execute: `"claude CLI not found in PATH"`.

### Claude Code Backend

```
claude -p "<prompt>" --dangerously-skip-permissions --output-format json [--resume <id>] [--append-system-prompt "..."] [--max-turns N]
```

Parse JSON response to extract `session_id` and `result`.

### Codex Backend

```
codex exec "<prompt>" --json --yolo [resume <id>]
```

Parse JSONL stream, extract final result from `turn.completed` event.

### Session Management

- In-memory `map[string]*Session` with mutex
- Auto-incrementing IDs: `as-1`, `as-2`, ...
- Max concurrent sessions: 8
- Async processes monitored via goroutine, status updated on exit
- Output capped at 1 MiB (reuse `limitedBuffer` pattern from `shellx`)

### Sync vs Async

- **Sync** (default): `Execute` blocks until CLI completes. Timeout via context (default 600s).
- **Async** (`async: true`): CLI runs in background goroutine. Returns immediately. Use `status` to poll.

## Error Handling

| Scenario | Behavior |
|----------|----------|
| CLI not installed | Return error: `"<backend> CLI not found in PATH"` |
| CLI timeout | Context deadline cancels process |
| CLI non-zero exit | Return stderr in error result |
| JSON parse failure | Fallback to raw stdout text |
| Session not found | Return `"session <id> not found"` |
| Max sessions reached | Return `"max sessions (8) reached, destroy one first"` |
| Async process crash | Goroutine updates status to `"failed"` with stderr |

## Security

- CLIs run with permission bypass (`--dangerously-skip-permissions` / `--yolo`) — intentional, as friday itself runs in a trusted environment
- `working_dir` validated against agent workspace (reuse `filex/guard.go` path check)
- Max concurrent sessions prevent resource exhaustion
- Output size capped at 1 MiB

## Future Extensions

1. **Provider mode**: Implement `claude-cli` provider reusing `Backend` abstraction
2. **tmux mode**: Add `TmuxBackend` implementing `Backend` interface for persistent TUI sessions
3. **Streaming**: Use `--output-format stream-json` for real-time progress in async mode
4. **Session persistence**: Persist `CLISessionID` mapping to disk for cross-restart resume
