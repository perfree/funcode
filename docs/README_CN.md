<div align="center">

# FunCode

**基于 Go 构建的终端 AI Coding Agent**

速度、结构、真正的工具调用。

[![Go](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](../LICENSE)

[English](../README.md) | [中文文档](README_CN.md)

</div>

---

## FunCode 是什么？

FunCode 是一个运行在终端中的 AI Coding Agent。它不只是聊天——它能读取你的代码、调用工具、切换角色、委托任务，并在实时 TUI 中展示整个过程。

**FunCode = 一个真正会调用工具的 AI 开发助手，而不只是一个聊天框。**

## 特性

- **多角色协作** — 通过 `@role` 在 `developer`、`architect` 等角色间切换，支持跨 Agent 委托与协作
- **内置工具** — 读取、写入、编辑、删除文件，执行 Shell 命令，搜索、文件匹配、差异对比、打补丁等
- **交互式 TUI** — 流式输出、工具执行块、角色活动展示、状态栏，基于 [Bubble Tea](https://github.com/charmbracelet/bubbletea) 构建
- **简单 CLI 模式** — 适合调试或 IDE 集成的轻量模式（`--simple`）
- **多模型支持** — 支持 OpenAI、Anthropic、Google Gemini 和 Ollama
- **技能系统** — 通过 `SKILL.md` 文件扩展能力
- **MCP 支持** — Model Context Protocol 协议集成
- **对话持久化** — 基于 SQLite 的历史记录，支持 `--resume` 恢复上次对话
- **细粒度权限** — 每个工具可独立配置 `allow` / `deny` / `ask` 权限
- **跨平台** — 支持 Windows、Linux 和 macOS（amd64 & arm64）

## 快速开始

### 前置要求

- [Go](https://go.dev/dl/) 1.24 或更高版本

### 安装

```bash
git clone https://github.com/perfree/funcode.git
cd funcode
make build
```

构建产物将输出到 `dist/funcode`。

### 运行

```bash
./dist/funcode
```

首次启动时，FunCode 会引导你完成交互式配置，设置模型和 API Key。

### 使用方式

```bash
funcode                          # TUI 模式启动（默认）
funcode --simple                 # 简单 CLI 模式启动
funcode --model gpt-4            # 指定模型
funcode --role architect         # 指定角色启动
funcode --resume                 # 恢复上次对话
funcode --config path/to/config  # 使用自定义配置文件
```

## 配置

FunCode 使用 YAML 配置文件：

| 路径 | 作用域 |
|------|--------|
| `~/.funcode/config.yaml` | 全局配置（用户级） |
| `.funcode/config.yaml` | 项目级配置 |

### 配置示例

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
    description: "高级开发工程师，处理编码任务"
    model: "gpt-4"
    tools: ["*"]
    prompt_file: "prompts/developer.md"

  - name: "architect"
    description: "软件架构师，设计系统和评审架构"
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

## 内置工具

| 工具 | 描述 | 默认权限 |
|------|------|----------|
| Bash | 执行 Shell 命令 | ask |
| Read | 读取文件内容 | allow |
| Write | 创建新文件 | ask |
| Edit | 编辑已有文件 | ask |
| Delete | 删除文件 | ask |
| Glob | 文件模式匹配 | allow |
| Grep | 搜索文件内容 | allow |
| Tree | 显示目录树 | allow |
| Diff | 文件差异对比 | allow |
| Patch | 应用补丁 | ask |
| Delegate | 委托任务给其他角色 | ask |
| Collaborate | 多智能体并行执行 | ask |

## 项目结构

```
funcode/
├── cmd/funcode/          # 程序入口
├── internal/
│   ├── agent/            # Agent 核心（角色、记忆、协作）
│   ├── tool/             # 工具系统与内置工具
│   ├── llm/              # LLM 提供商（OpenAI、Anthropic、Google、Ollama）
│   ├── config/           # 配置管理
│   ├── tui/              # 终端界面（Bubble Tea）
│   ├── skill/            # 技能系统
│   ├── conversation/     # 对话持久化（SQLite）
│   ├── mcp/              # MCP 协议支持
│   └── plan/             # 规划模块
├── configs/              # 配置示例与 Prompt 文件
├── Makefile
└── go.mod
```

## 构建

```bash
make build          # 构建当前平台
make build-all      # 构建所有平台（Windows/Linux/macOS）
make run            # 直接运行
make test           # 运行测试
make clean          # 清理构建产物
```

## 开源协议

本项目基于 [MIT License](../LICENSE) 开源。
