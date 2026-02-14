# Friday

**Thank God It's Friday, Your Personal AI Assistant**

Friday is a Go-based multi-agent framework inspired by projects like OpenClaw and Nanobot.

This repository is currently a **prototype**:
- Core architecture is in place.
- Key flows are working end-to-end.
- APIs and module boundaries may still change quickly.

## What Friday Does

- Routes messages from multiple channels to the right agent.
- Supports a brain-agent mode and team-agent mode.
- Integrates multiple LLM providers with fallback support.
- Runs tool calls inside the agent loop.
- Uses workspace context and skills to guide behavior.

## Quick Start

### 1. Prerequisites

- Go 1.24+

### 2. Prepare config

```bash
cp conf/config.yaml.example conf/config.yaml
```

Edit `conf/config.yaml` and set:
- Providers (API keys/endpoints)
- Primary model (`provider_id:model_name`)
- Channels and agents

### 3. Run Friday

```bash
go run . serve -c conf/config.yaml
```

## Current Prototype Scope

- Config-driven startup and routing
- Agent loop with tool-calling
- Multi-provider runtime abstraction
- Telegram and Lark channel adapters

## Project Layout

- `main.go`: CLI entrypoint
- `conf/`: config schema and examples
- `internal/gateway/`: startup, routing, dispatch
- `internal/agent/`: agent runtime, loop, session
- `internal/provider/`: model provider implementations
- `internal/channel/`: channel implementations
- `workspace/`: prompt and runtime context files
- `skills/`: built-in skills

## Roadmap (Prototype -> Stable)

### P0: Core Reliability

- Stabilize session key strategy across DM and group scenarios.
- Add durable session persistence and restart recovery.
- Enforce bounded history windows in runtime context.
- Improve provider fallback behavior and error surfaces.

### P1: Agent Quality

- Add tool-result pruning and smarter context shaping.
- Introduce memory compaction and long-term memory hygiene.
- Improve multi-agent collaboration and task delegation quality.
- Expand built-in skills and tool integration coverage.

### P2: Production Readiness

- Add better observability (structured logs, metrics, trace hooks).
- Harden channel adapters and failure recovery paths.
- Improve configuration validation and operator UX.
- Publish release process and compatibility guarantees.

## Star History

[![Star History Chart](https://api.star-history.com/svg?repos=TGIFAI/friday&type=Date)](https://star-history.com/#TGIFAI/friday&Date)

## Status

Friday is under active development. Expect rapid iteration and occasional breaking changes.

