package builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"time"

	"github.com/perfree/funcode/internal/tool"
)

type BashTool struct{}

type bashParams struct {
	Command string `json:"command"`
	Timeout int    `json:"timeout,omitempty"` // milliseconds
}

func NewBashTool() *BashTool { return &BashTool{} }

func (t *BashTool) Name() string { return "Bash" }
func (t *BashTool) Description() string {
	return `Execute a shell command and return its output.

Usage:
- Use for builds, tests, git operations, package management, and other system commands.
- Do NOT use Bash for tasks that have dedicated tools: use Read (not cat), Glob (not ls/find), Grep (not grep), Edit (not sed).
- Set timeout for long-running commands (default: 120 seconds).`
}

func (t *BashTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"command": {
				"type": "string",
				"description": "The shell command to execute"
			},
			"timeout": {
				"type": "integer",
				"description": "Timeout in milliseconds (default: 120000)"
			}
		},
		"required": ["command"]
	}`)
}

func (t *BashTool) RequiresApproval(_ json.RawMessage) bool { return true }

func (t *BashTool) Execute(ctx context.Context, params json.RawMessage) (*tool.Result, error) {
	var p bashParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	timeout := 120 * time.Second
	if p.Timeout > 0 {
		timeout = time.Duration(p.Timeout) * time.Millisecond
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", p.Command)
	} else {
		cmd = exec.CommandContext(ctx, "bash", "-c", p.Command)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := stdout.String()
	if stderr.Len() > 0 {
		if output != "" {
			output += "\n"
		}
		output += stderr.String()
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return &tool.Result{Error: "command timed out"}, nil
		}
		// Include output in Content even on error so AI can see what went wrong
		return &tool.Result{
			Content: output,
			Error:   fmt.Sprintf("command failed: %v", err),
		}, nil
	}

	return &tool.Result{Content: output}, nil
}
