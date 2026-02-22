<p align="center">
  <h1 align="center">Friday</h1>
  <p align="center"><b>Thank God It's Friday — 你的私人 AI 助手</b></p>
</p>

<p align="center">
  <a href="https://github.com/TGIFAI/friday/releases"><img src="https://img.shields.io/github/v/release/TGIFAI/friday?include_prereleases&label=release" alt="Release"></a>
  <a href="https://github.com/TGIFAI/friday/actions"><img src="https://img.shields.io/github/actions/workflow/status/TGIFAI/friday/release.yaml?label=CI" alt="CI"></a>
  <a href="https://goreportcard.com/report/github.com/tgifai/friday"><img src="https://goreportcard.com/badge/github.com/tgifai/friday" alt="Go Report Card"></a>
  <img src="https://img.shields.io/badge/go-%3E%3D1.24-blue" alt="Go 1.24+">
</p>

<p align="center">
  <a href="https://tgif.sh">官网</a> · <a href="https://docs.tgif.sh">文档</a> · <a href="#快速开始">快速开始</a> · <a href="#配置">配置</a> · <a href="./README.md">English</a>
</p>

---

Friday 是一个自托管的多智能体 AI 助手，使用 Go 编写。它将你常用的聊天平台连接到 LLM 提供方，运行自主的智能体循环，支持工具执行、持久化记忆和技能系统 —— 所有配置通过一个 YAML 文件驱动。

## 核心特性

- **多渠道** — 开箱即用支持 Telegram、飞书（Lark）和 HTTP API。每个渠道处理平台特有的细节（媒体组、@提及、表情回应），智能体看到的是统一、干净的消息流。
- **多模型供应商 + 故障切换** — 支持 OpenAI、Anthropic、Gemini、Ollama、通义千问。可为每个智能体配置主模型和兜底链，故障时自动切换。
- **智能体工具循环** — 智能体迭代调用工具直到任务完成。内置工具族：Shell 执行、文件操作、网页搜索与抓取、定时任务管理和消息发送。
- **双层记忆** — `MEMORY.md` 中的持久知识 + `memory/daily/` 中的每日事件日志。夜间压缩任务自动精简每日日志并提升持久性事实。
- **技能系统** — YAML + Markdown 格式的行为扩展（类似系统提示词插件）。内置 GitHub、Notion、Obsidian、tmux、摘要总结等技能，支持按智能体或全局添加自定义技能。
- **工作区驱动的人格** — 每个智能体拥有一组 Markdown 模板（SOUL、IDENTITY、TOOLS、SECURITY……）来塑造其系统提示词，完全可定制。
- **安全与访问控制** — 按渠道配置配对策略（`welcome` / `silent` / `custom`），以及群组/用户级别的白名单和黑名单。
- **定时任务** — 内置 cron 调度器，支持心跳检查、记忆压缩和自定义周期性任务。

## 支持的平台

| 聊天渠道 | LLM 供应商 |
|:---:|:---:|
| Telegram | OpenAI |
| 飞书（Lark） | Anthropic |
| HTTP API | Gemini |
| | Ollama |
| | 通义千问（Qwen） |

## 快速开始

### 1. 安装

```bash
# 从源码构建（需要 Go 1.24+）
git clone https://github.com/TGIFAI/friday.git
cd friday
go build -trimpath -ldflags="-s -w" -o friday ./cmd/friday
```

### 2. 初始化

交互式引导程序会创建配置文件和工作区：

```bash
./friday onboard
```

它会询问你的 LLM 供应商凭证和聊天渠道令牌，然后生成 `~/.friday/config.yaml` 和智能体工作区。

### 3. 运行

```bash
./friday gateway run
```

Friday 启动 HTTP 服务器，连接到已配置的渠道，开始监听消息。

## 配置

Friday 通过一个 YAML 文件进行配置。运行 `friday onboard` 可交互式生成，或复制示例文件：

```bash
cp config.yaml.example ~/.friday/config.yaml
```

主要配置段：

```yaml
# LLM 供应商（API 密钥、端点、模型）
providers:
  openai-main:
    type: "openai"
    config:
      api_key: "${OPENAI_API_KEY}"
      default_model: "gpt-4o-mini"

# 智能体（模型路由、工作区、技能）
agents:
  default:
    channels: ["telegram-main"]
    models:
      primary: "openai-main:gpt-4o-mini"
      fallback: ["openai-main:gpt-4.1-mini"]

# 聊天渠道（平台凭证、安全策略）
channels:
  telegram-main:
    type: "telegram"
    enabled: true
    config:
      token: "${TELEGRAM_BOT_TOKEN}"
```

完整配置项请参考 [`config.yaml.example`](config.yaml.example)。

## CLI 命令

| 命令 | 说明 |
|---------|-------------|
| `friday onboard` | 交互式首次配置引导 |
| `friday gateway run` | 启动网关运行时 |
| `friday msg` | 通过渠道发送单条消息 |
| `friday cronjob list` | 列出所有持久化的定时任务 |
| `friday update` | 从 GitHub Releases 检查并应用更新 |

## 架构

```
用户 ──► 渠道（Telegram / 飞书 / HTTP）
              │
              ▼
           网关 ──► 安全层（ACL + 配对）
              │             │
              │       命令路由 ──► /start, /help, ...
              │
              ▼
         智能体循环 ──► 供应商（OpenAI / Anthropic / ...）
              │                │
              │          LLM 生成
              │                │
              ▼                ▼
         工具执行器   ◄── 工具调用
         (shell, 文件, 网页, 定时任务, 消息)
              │
              ▼
         会话 + 记忆
         (JSONL 历史, 每日日志, MEMORY.md)
```

### 项目结构

```
cmd/friday/              CLI 入口和子命令
internal/
  gateway/               HTTP 服务器、路由、消息队列、安全
  agent/                 智能体运行时、循环、会话、上下文构建
    tool/                内置工具（shellx, filex, webx, cronx, msgx, qmdx）
    skill/               技能注册和加载器
    session/             JSONL 会话持久化
  channel/               渠道接口和实现
    telegram/            Telegram 适配器（轮询 + Webhook）
    lark/                飞书适配器（WebSocket + Webhook）
    http/                HTTP API 适配器
  provider/              供应商接口和实现
    openai/              OpenAI（+ 兼容 API）
    anthropic/           Anthropic Claude
    gemini/              Google Gemini
    ollama/              Ollama（本地模型）
    qwen/                通义千问
  cronjob/               定时调度器、心跳、记忆压缩
  config/                配置模式、解析、校验
  consts/                常量、工作区模板
```

## Star 历史

[![Star History Chart](https://api.star-history.com/svg?repos=TGIFAI/friday&type=Date)](https://star-history.com/#TGIFAI/friday&Date)
