# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Friday is a Go-based multi-agent framework that routes messages from channels (Telegram, Lark) to LLM-powered agents. It supports multiple LLM providers with fallback, tool execution within an agent loop, and a skills system for behavioral extensions.

## Build & Run

```bash
# Build
go build -trimpath -ldflags="-s -w" ./cmd/friday

# Run
go run . serve -c config.yaml

# Test all
go test ./...

# Test single package
go test ./internal/provider/openai/...

# Test verbose
go test -v -run TestFunctionName ./path/to/package/...
```

Note: Some tests require environment variables (e.g., `OPENAI_API_KEY`) and are skipped if not set.

## Architecture

### Provider System (`internal/provider/`)

The core abstraction is the **Provider interface** (`interface.go`) with methods: `ID()`, `Type()`, `IsAvailable()`, `ListModels()`, `Generate()`, `Stream()`, `Close()`.

- **Registry** (`registry.go`): Thread-safe singleton (sync.RWMutex) for registering/retrieving providers. Uses double-checked locking for lazy model instance creation.
- **Implementations**: OpenAI, Anthropic, Gemini, Ollama, Qwen — each under `internal/provider/<name>/`. All built on the **Bytedance Eino framework** (`github.com/cloudwego/eino` and `eino-ext` packages).
- **Config pattern**: Each provider has a `ParseConfig()` function with validation and defaults (timeout: 60s, retries: 3). Supports env var overrides for `api_key`, `base_url`, `default_model`, etc.
- OpenAI provider includes a background **heartbeat** health check (30s interval).

### Logging (`internal/pkg/logs/`)

Context-aware structured logging built on logrus. Supports LogID propagation, JSON/text formatting, stdout/file/dual output, and file rotation via lumberjack.

### Skills (`skills/`)

Behavioral extensions in YAML frontmatter + Markdown format. Built-in skills: GitHub CLI, Notion, Obsidian, tmux, summarization, skill authoring.

### Configuration

See `config.yaml.example` for structure. Key sections: `gateway` (bind address, concurrency, timeout) and `logging` (level, format, output, rotation).

## CI/CD

GitHub Actions (`release.yaml`) triggers on git tags. Builds cross-platform binaries (linux/darwin amd64+arm64, windows amd64) with version injection via ldflags (`DATE`, `GIT_SHA`).

## Module

Go 1.24.0 — module path: `github.com/tgifai/friday`
