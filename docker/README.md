# Docker

## Quick Start

1. Build and start:

```bash
cd docker
docker compose up -d
```

2. Run the interactive setup wizard to initialize config:

```bash
docker compose exec -it friday friday onboard
```

The wizard walks you through:

- Accepting the disclaimer
- Selecting an LLM provider (OpenAI, Anthropic, Gemini, Ollama, Qwen)
- Entering API key and model
- Choosing a channel (Telegram, Lark, HTTP)
- Enabling auto-update

Once complete, config is written to `/root/.friday/config.yaml` inside the container (persisted via the `friday-home` volume).

3. Restart to pick up the new config:

```bash
docker compose restart
```

4. Check logs:

```bash
docker compose logs -f
```

## Using an Existing Config

If you already have a `config.yaml`, mount it directly instead of running onboard:

```yaml
# docker-compose.yml
services:
  friday:
    volumes:
      - friday-home:/root/.friday
      - ./config.yaml:/root/.friday/config.yaml
```

## Shell Access

The container ships with fish as the default shell:

```bash
docker compose exec -it friday fish
```

## Data Persistence

All friday data lives under `/root/.friday` inside the container, mapped to the `friday-home` named volume:

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
