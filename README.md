# FunCode

> Terminal-native AI coding agent for builders who want speed, structure, and real tool use.

[GitHub](https://github.com/perfree/funcode) · [中文](#中文)

FunCode is an AI coding agent built with Go for the terminal.
It can read code, inspect projects, call tools, switch roles, delegate work, and show the whole process in a clean TUI.

---

## What Is FunCode

FunCode is built for workflows like:

- understanding an unfamiliar codebase fast
- switching between roles like `developer` and `architect`
- reading files, editing code, and running tasks in the terminal
- extending behavior with `SKILL.md`
- watching tool calls, streaming output, and role collaboration in real time

**FunCode = a real tool-using AI development assistant, not just a chat box.**

---

## Highlights

- Multi-role collaboration
- Tool-driven execution
- TUI + simple CLI mode
- `SKILL.md` support
- Streaming UI with live blocks and timing
- Configurable models, roles, prompts, and permissions

---

## Quick Start

### 1. Clone

```bash
git clone https://github.com/perfree/funcode.git
cd funcode
```

### 2. Build

```bash
go build ./cmd/funcode
```

### 3. Run

```bash
go run ./cmd/funcode/
```

On first launch, FunCode will guide you through setup.

---

## Configuration

Default paths:

- Global config: `~/.funcode/`
- Project config: `.funcode/`

You can configure:

- model and provider
- `base_url`
- roles
- prompt files
- tool permissions
- skills

---

## Examples

```text
Let @architect inspect the project and give me a summary
```

```text
Check the current repo structure, then fix the startup error
```

```text
Use Agent in parallel to inspect README and go.mod
```

---

## Run Modes

### TUI

Best for daily use:

- streaming output
- tool blocks
- role collaboration blocks
- status line

### Simple

Best for debugging or IDE runs:

```bash
go run ./cmd/funcode/ --simple
```

---

## Philosophy

FunCode focuses on:

- real coding workflows
- clear multi-role collaboration
- dense terminal feedback
- less ceremony, more action

---

## License

See repository for license details.

---

## 中文

[English](#funcode)

FunCode 是一个基于 Go 构建的终端 AI Coding Agent。
它不只是聊天，而是真的可以读代码、看项目、调用工具、切换角色、委托协作，并把整个过程用 TUI 展示出来。

---

## FunCode 是什么

它适合这些场景：

- 快速看懂一个陌生项目
- 在 `developer`、`architect` 等角色之间切换协作
- 在终端里直接读文件、改代码、执行任务
- 用 `SKILL.md` 扩展能力
- 实时观察工具调用、流式输出和角色协作过程

**FunCode = 一个真正会调用工具的 AI 开发助手，而不只是一个聊天框。**

---

## 特性

- 多角色协作
- 工具驱动执行
- TUI + simple CLI 模式
- 支持 `SKILL.md`
- 支持流式 UI、实时块和耗时展示
- 模型、角色、提示词、权限均可配置

---

## 快速开始

### 1. 克隆项目

```bash
git clone https://github.com/perfree/funcode.git
cd funcode
```

### 2. 构建

```bash
go build ./cmd/funcode
```

### 3. 运行

```bash
go run ./cmd/funcode/
```

首次启动时，FunCode 会引导你完成基础配置。

---

## 配置目录

默认路径：

- 全局配置：`~/.funcode/`
- 项目配置：`.funcode/`

可配置内容包括：

- 模型和 provider
- `base_url`
- 角色
- prompt 文件
- 工具权限
- skills

---

## 示例

```text
让@architect 看下项目，总结一下
```

```text
检查当前仓库结构，然后修复启动报错
```

```text
请并行使用 Agent 分析 README 和 go.mod
```

---

## 运行模式

### TUI

适合日常使用：

- 流式输出
- 工具块展示
- 角色协作块
- 底部状态栏

### Simple

适合调试或在 IDE 中直接运行：

```bash
go run ./cmd/funcode/ --simple
```

---

## 设计方向

FunCode 关注的是：

- 真正可执行的 coding workflow
- 清晰的多角色协作体验
- 终端中的高密度反馈
- 少废话，多行动

---

## License

许可证信息请以仓库实际内容为准。
