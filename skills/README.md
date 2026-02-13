# Friday Skills

This directory contains built-in skills that extend Friday's capabilities.

## Skill Format

Each skill is a directory containing a `SKILL.md` file with:
- YAML frontmatter (for example: `name`, `description`, `metadata`)
- Markdown instructions for the agent
- Optional `scripts/`, `assets/`, or `references/` folders when needed

## Attribution

Many skills in this directory are adapted from or inspired by
[OpenClaw](https://github.com/openclaw/openclaw)'s skill system.
The skill format and metadata structure follow OpenClaw-style conventions
to keep skill authoring and migration straightforward.

## Available Skills

Friday currently supports the following built-in skills:

| Skill | Description |
|-------|-------------|
| `chinese-copywriting` | Edit and normalize Chinese copywriting |
| `github` | Interact with GitHub using the `gh` CLI |
| `notion` | Manage Notion pages, databases, and blocks |
| `obsidian` | Work with Obsidian vault notes |
| `skill-creator` | Create or update skills |
| `summarize` | Summarize URLs, podcasts, videos, and local files |
| `tmux` | Remote-control tmux sessions |

For the full list, browse subdirectories in `skills/`.

