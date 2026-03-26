package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"

	"github.com/perfree/funcode/internal/tool"
)

type RunTaskTool struct{}

type runTaskParams struct {
	Task    string `json:"task,omitempty"`
	Command string `json:"command"`
	Cwd     string `json:"cwd,omitempty"`
	Timeout int    `json:"timeout,omitempty"`
}

func NewRunTaskTool() *RunTaskTool { return &RunTaskTool{} }

func (t *RunTaskTool) Name() string { return "RunTask" }
func (t *RunTaskTool) Description() string {
	return "Run a verification task such as build, test, or lint and return a structured result with task name, exit code, duration, and output."
}

func (t *RunTaskTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"task": {
				"type": "string",
				"description": "Short task label such as build, test, lint, or verify"
			},
			"command": {
				"type": "string",
				"description": "Shell command to run"
			},
			"cwd": {
				"type": "string",
				"description": "Optional working directory"
			},
			"timeout": {
				"type": "integer",
				"description": "Timeout in milliseconds (default: 120000)"
			}
		},
		"required": ["command"]
	}`)
}

func (t *RunTaskTool) RequiresApproval(_ json.RawMessage) bool { return true }

func (t *RunTaskTool) Execute(ctx context.Context, params json.RawMessage) (*tool.Result, error) {
	var p runTaskParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	taskName := p.Task
	if taskName == "" {
		taskName = "task"
	}
	cwd := p.Cwd
	if cwd == "" {
		cwd, _ = os.Getwd()
	}

	var commandName string
	var args []string
	if runtime.GOOS == "windows" {
		commandName = "cmd"
		args = []string{"/C", p.Command}
	} else {
		commandName = "bash"
		args = []string{"-c", p.Command}
	}

	result, err := runCommandWithInput(ctx, cwd, commandName, args, "", p.Timeout)
	if result.TimedOut {
		return &tool.Result{
			Content: formatRunTaskOutput(taskName, p.Command, cwd, result.ExitCode, result.Duration.String(), result.Combined),
			Error:   "task timed out",
		}, nil
	}

	content := formatRunTaskOutput(taskName, p.Command, cwd, result.ExitCode, result.Duration.String(), result.Combined)
	if err != nil {
		return &tool.Result{
			Content: content,
			Error:   fmt.Sprintf("task failed with exit code %d", result.ExitCode),
		}, nil
	}

	return &tool.Result{Content: content}, nil
}

func formatRunTaskOutput(task string, command string, cwd string, exitCode int, duration string, output string) string {
	content := fmt.Sprintf("Task: %s\nCommand: %s\nDirectory: %s\nExit Code: %d\nDuration: %s", task, command, cwd, exitCode, duration)
	if output != "" {
		content += "\nOutput:\n" + output
	}
	return content
}
