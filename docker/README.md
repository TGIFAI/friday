# Docker

## Quick Start

### 1. Build the image

```bash
cd docker
docker compose build
```

### 2. Run the setup wizard

The container needs a config before it can start. Use a temporary container to run onboard:

```bash
docker compose run --rm friday onboard
```

The wizard walks you through:

- Accepting the disclaimer
- Selecting an LLM provider (OpenAI, Anthropic, Gemini, Ollama, Qwen)
- Entering API key and model
- Choosing a channel (Telegram, Lark, HTTP)
- Enabling auto-update

Config is written to the `friday-home` volume at `/root/.friday/config.yaml`.

### 3. Start the service

```bash
docker compose up -d
```

### 4. Check logs

```bash
docker compose logs -f
```

## Using an Existing Config

Skip onboard entirely by mounting your own config:

```yaml
# docker-compose.yml
services:
  friday:
    volumes:
      - friday-home:/root/.friday
      - ./config.yaml:/root/.friday/config.yaml
```

## Re-running Onboard

To reconfigure at any time, stop the service and run onboard again:

```bash
docker compose down
docker compose run --rm friday onboard
docker compose up -d
```

## Shell Access

```bash
docker compose exec -it friday fish
```

## Data Persistence

All friday data lives under `/root/.friday`, persisted in the `friday-home` named volume:

```
/root/.friday/
├── config.yaml          # runtime config
├── skills/              # global skills (cloned during onboard)
├── workspaces/          # agent workspaces, memory, sessions
└── logs/                # log files
```

Removing the container does **not** delete this data. To fully reset:

```bash
docker compose down -v
```

## Build Args

| Arg | Default | Description |
|-----|---------|-------------|
| `VERSION` | `dev` | Version string injected into the binary |

## Ports

| Port | Description |
|------|-------------|
| `8088` | Gateway HTTP server |

## Included Tools

The runtime image (Ubuntu 24.04) includes:

| Category | Packages |
|----------|----------|
| Shell & Editor | `fish`, `vim` |
| Network | `curl`, `wget`, `mtr`, `ping`, `dig`, `net-tools`, `iproute2`, `ssh` |
| Dev | `git`, `jq`, `tree`, `unzip`, `zip` |
| Python | `python3`, `pip3`, `python3-venv` |
| System | `htop`, `ps`, `lsof`, `strace`, `less`, `file` |
