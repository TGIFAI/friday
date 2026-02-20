<p align="center">
  <h1 align="center">Friday</h1>
  <p align="center"><b>Thank God It's Friday — Your Personal AI Assistant</b></p>
</p>

<p align="center">
  <a href="https://github.com/TGIFAI/friday/releases"><img src="https://img.shields.io/github/v/release/TGIFAI/friday?include_prereleases&label=release" alt="Release"></a>
  <a href="https://github.com/TGIFAI/friday/actions"><img src="https://img.shields.io/github/actions/workflow/status/TGIFAI/friday/release.yaml?label=CI" alt="CI"></a>
  <a href="https://goreportcard.com/report/github.com/tgifai/friday"><img src="https://goreportcard.com/badge/github.com/tgifai/friday" alt="Go Report Card"></a>
  <img src="https://img.shields.io/badge/go-%3E%3D1.24-blue" alt="Go 1.24+">
</p>

<p align="center">
  <a href="https://tgif.sh">Website</a> · <a href="https://docs.tgif.sh">Docs</a> · <a href="#quick-start">Getting Started</a> · <a href="#configuration">Configuration</a>
</p>

---

Friday is a self-hosted, multi-agent AI assistant written in Go. It connects your favorite chat platforms to LLM providers and runs an autonomous agent loop with tool execution, persistent memory, and a skills system — all driven by a single YAML config file.

## Key Features

- **Multi-Channel** — Telegram, Lark (Feishu), and HTTP API out of the box. Each channel handles platform-specific details (media groups, mentions, reactions) so the agent sees a clean, unified message stream.
- **Multi-Provider with Fallback** — OpenAI, Anthropic, Gemini, Ollama, Qwen. Configure a primary model and fallback chain per agent; Friday switches automatically on failure.
- **Agentic Tool Loop** — Agents call tools iteratively until the task is done. Built-in tool families: shell execution, file operations, web search & fetch, cron management, and messaging.
- **Two-Tier Memory** — Persistent knowledge in `MEMORY.md` + daily event logs in `memory/daily/`. A nightly compaction job automatically condenses daily logs and promotes durable facts.
- **Skills System** — Behavioral extensions in YAML + Markdown (like system prompt plugins). Built-in skills for GitHub, Notion, Obsidian, tmux, summarization, and more. Add your own per-agent or globally.
- **Workspace-Driven Personality** — Each agent has a workspace of Markdown templates (SOUL, IDENTITY, TOOLS, SECURITY, …) that shape its system prompt. Fully customizable.
- **Security & ACL** — Per-channel pairing policies (`welcome` / `silent` / `custom`) and group/user-level allow/block lists.
- **Scheduled Jobs** — Built-in cron scheduler for heartbeat checks, memory compaction, and custom recurring tasks.

## Supported Platforms

| Chat Channels | LLM Providers |
|:---:|:---:|
| Telegram | OpenAI |
| Lark (Feishu) | Anthropic |
| HTTP API | Gemini |
| | Ollama |
| | Qwen |

## Quick Start

### 1. Install

```bash
# Build from source (requires Go 1.24+)
git clone https://github.com/TGIFAI/friday.git
cd friday
go build -trimpath -ldflags="-s -w" -o friday ./cmd/friday
```

### 2. Onboard

The interactive setup wizard creates your config and workspace:

```bash
./friday onboard
```

It will ask for your LLM provider credentials and chat channel tokens, then generate `~/.friday/config.yaml` and the agent workspace.

### 3. Run

```bash
./friday gateway run
```

Friday starts the HTTP server, connects to your configured channels, and begins listening for messages.

## Configuration

Friday is configured via a single YAML file. Run `friday onboard` to generate one interactively, or copy the example:

```bash
cp config.yaml.example ~/.friday/config.yaml
```

Key sections:

```yaml
# LLM providers (API keys, endpoints, models)
providers:
  openai-main:
    type: "openai"
    config:
      api_key: "${OPENAI_API_KEY}"
      default_model: "gpt-4o-mini"

# Agents (model routing, workspace, skills)
agents:
  default:
    channels: ["telegram-main"]
    models:
      primary: "openai-main:gpt-4o-mini"
      fallback: ["openai-main:gpt-4.1-mini"]

# Chat channels (platform credentials, security)
channels:
  telegram-main:
    type: "telegram"
    enabled: true
    config:
      token: "${TELEGRAM_BOT_TOKEN}"
```

See [`config.yaml.example`](config.yaml.example) for the full reference with all options.

## CLI Commands

| Command | Description |
|---------|-------------|
| `friday onboard` | Interactive first-time setup wizard |
| `friday gateway run` | Start the gateway runtime |
| `friday msg` | Send a one-off message through a channel |
| `friday cronjob list` | List all persisted cron jobs |
| `friday update` | Check for and apply updates from GitHub releases |

## Architecture

```
User ──► Channel (Telegram / Lark / HTTP)
              │
              ▼
         Gateway ──► Security (ACL + Pairing)
              │             │
              │     Command Router ──► /start, /help, ...
              │
              ▼
         Agent Loop ──► Provider (OpenAI / Anthropic / ...)
              │                │
              │          LLM Generate
              │                │
              ▼                ▼
         Tool Executor   ◄── Tool Calls
         (shell, file, web, cron, msg)
              │
              ▼
         Session + Memory
         (JSONL history, daily logs, MEMORY.md)
```

### Project Layout

```
cmd/friday/              CLI entrypoint and subcommands
internal/
  gateway/               HTTP server, routing, message queue, security
  agent/                 Agent runtime, loop, session, context building
    tool/                Built-in tools (shellx, filex, webx, cronx, msgx, qmdx)
    skill/               Skill registry and loader
    session/             JSONL session persistence
  channel/               Channel interface and implementations
    telegram/            Telegram adapter (polling + webhook)
    lark/                Lark/Feishu adapter (WebSocket + webhook)
    http/                HTTP API adapter
  provider/              Provider interface and implementations
    openai/              OpenAI (+ compatible APIs)
    anthropic/           Anthropic Claude
    gemini/              Google Gemini
    ollama/              Ollama (local models)
    qwen/                Alibaba Qwen
  cronjob/               Cron scheduler, heartbeat, memory compaction
  config/                Config schema, parsing, validation
  consts/                Constants, workspace templates
```

## Star History

[![Star History Chart](https://api.star-history.com/svg?repos=TGIFAI/friday&type=Date)](https://star-history.com/#TGIFAI/friday&Date)
