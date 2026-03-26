<div align="center">

# FunCode

**Terminal-native AI Coding Agent built with Go.**

Speed. Structure. Real tool use.

[![Go](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

[English](README.md) | [中文文档](docs/README_CN.md)

</div>

---

## What is FunCode?

FunCode is an AI coding agent that runs in your terminal. It doesn't just chat — it reads your code, calls tools, switches between roles, delegates tasks, and shows the entire process in a real-time TUI.

**FunCode = a real tool-using AI development assistant, not just a chat box.**

## Features

- **Multi-Role Collaboration** — Switch between roles like `developer` and `architect` with `@role`, delegate and collaborate across agents
- **Built-in Tools** — Read, write, edit, delete files, run shell commands, grep, glob, diff, patch, and more
- **Interactive TUI** — Streaming output, tool execution blocks, role activity display, and status bar powered by [Bubble Tea](https://github.com/charmbracelet/bubbletea)
- **Simple CLI Mode** — Lightweight mode for debugging or IDE integration (`--simple`)
- **Multi-Provider LLM** — Supports OpenAI, Anthropic, Google Gemini, and Ollama
- **Skill System** — Extend capabilities with `SKILL.md` files
- **MCP Support** — Model Context Protocol integration
- **Conversation Persistence** — SQLite-based history with `--resume` to continue previous sessions
- **Fine-grained Permissions** — Control tool access with `allow` / `deny` / `ask` per tool
- **Cross-Platform** — Builds for Windows, Linux, and macOS (amd64 & arm64)

## Quick Start

### Prerequisites

- [Go](https://go.dev/dl/) 1.24 or later

### Install

```bash
git clone https://github.com/perfree/funcode.git
cd funcode
make build
```

The binary will be output to `dist/funcode`.

### Run

```bash
./dist/funcode
```

On first launch, FunCode will guide you through an interactive setup to configure your model and API key.

### Usage

```bash
funcode                          # Start with TUI mode (default)
funcode --simple                 # Start in simple CLI mode
funcode --model gpt-4            # Specify a model
funcode --role architect         # Start with a specific role
funcode --resume                 # Resume the last conversation
funcode --config path/to/config  # Use a custom config file
```

## Configuration

FunCode uses YAML configuration files:

| Path | Scope |
|------|-------|
| `~/.funcode/config.yaml` | Global (user-level) |
| `.funcode/config.yaml` | Project-level |

### Example

```yaml
models:
  - name: "gpt-4"
    provider: "openai"
    model: "gpt-4"
    api_key: "sk-xxxx"
    base_url: "https://api.openai.com/v1"
    max_tokens: 8192

roles:
  - name: "developer"
    description: "Senior developer for coding tasks"
    model: "gpt-4"
    tools: ["*"]
    prompt_file: "prompts/developer.md"

  - name: "architect"
    description: "Software architect for design and review"
    model: "gpt-4"
    tools: ["Tree", "Read", "ReadRange", "Glob", "Grep", "Diff"]
    prompt_file: "prompts/architect.md"

default_role: "developer"

permissions:
  - tool: "Read"
    action: "allow"
  - tool: "Bash"
    action: "ask"
  - tool: "Write"
    action: "ask"

settings:
  theme: "dark"
  markdown_render: true
  auto_compact: true
```

## Built-in Tools

| Tool | Description | Default Permission |
|------|-------------|-------------------|
| Bash | Execute shell commands | ask |
| Read | Read file contents | allow |
| Write | Create new files | ask |
| Edit | Edit existing files | ask |
| Delete | Delete files | ask |
| Glob | File pattern matching | allow |
| Grep | Search file contents | allow |
| Tree | Display directory tree | allow |
| Diff | Compare file differences | allow |
| Patch | Apply patches | ask |
| Delegate | Delegate tasks to other roles | ask |
| Collaborate | Multi-agent parallel execution | ask |

## Project Structure

```
funcode/
├── cmd/funcode/          # Entry point
├── internal/
│   ├── agent/            # Agent core (roles, memory, collaboration)
│   ├── tool/             # Tool system and built-in tools
│   ├── llm/              # LLM providers (OpenAI, Anthropic, Google, Ollama)
│   ├── config/           # Configuration management
│   ├── tui/              # Terminal UI (Bubble Tea)
│   ├── skill/            # Skill system
│   ├── conversation/     # Conversation persistence (SQLite)
│   ├── mcp/              # MCP protocol support
│   └── plan/             # Planning module
├── configs/              # Example configs and prompt files
├── Makefile
└── go.mod
```

## Build

```bash
make build          # Build for current platform
make build-all      # Build for all platforms (Windows/Linux/macOS)
make run            # Run directly
make test           # Run tests
make clean          # Clean build output
```

## License

This project is licensed under the [MIT License](LICENSE).
