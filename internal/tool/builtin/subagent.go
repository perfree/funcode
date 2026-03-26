package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/perfree/funcode/internal/tool"
)

// SubAgentFunc is the function that runs a sub-agent with a task
type SubAgentFunc func(ctx context.Context, task string, description string) (string, error)

// SubAgentTool allows an agent to spawn a sub-agent for parallel task execution
type SubAgentTool struct {
	runFn SubAgentFunc
}

// NewSubAgentTool creates a sub-agent tool
func NewSubAgentTool(runFn SubAgentFunc) *SubAgentTool {
	return &SubAgentTool{runFn: runFn}
}

type subAgentParams struct {
	Task        string              `json:"task"`
	Description string              `json:"description"`
	Tasks       []subAgentTaskParam `json:"tasks"`
}

type subAgentTaskParam struct {
	Task        string `json:"task"`
	Description string `json:"description"`
}

func (t *SubAgentTool) Name() string { return "Agent" }

func (t *SubAgentTool) Description() string {
	return `Launch a worker sub-agent to handle a task autonomously. The worker has access to the normal file/code tools (Read, Write, Edit, Bash, Glob, Grep, etc.) and can work independently.

Use this tool when:
- You need to perform multiple independent tasks in parallel (e.g., reading several files, running multiple searches)
- A task is complex enough to benefit from isolated context
- You want to delegate a sub-task without polluting the main conversation context

**Parallel execution**: When you call multiple Agent tools in the same turn, they run in parallel automatically.
**Important**: For the most reliable parallel execution, you can call Agent once with a tasks array so the workers launch together inside one tool call.
**Important**: If the user asks for parallel work across independent items, launch all needed workers at once.

**Important**: The sub-agent has NO memory of the current conversation. Provide all necessary context in the task description.
**Important**: Worker sub-agents do not recursively call Agent/Delegate/Collaborate.

Example: Call Agent twice in one turn to search two different areas simultaneously.`
}

func (t *SubAgentTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"task": {
				"type": "string",
				"description": "A clear, self-contained description of the task for the sub-agent. Include all necessary context."
			},
			"description": {
				"type": "string",
				"description": "A short (3-5 word) summary of what the sub-agent will do, shown in the UI."
			},
			"tasks": {
				"type": "array",
				"description": "Optional batch mode. Each item launches one worker sub-agent in parallel.",
				"items": {
					"type": "object",
					"properties": {
						"task": {
							"type": "string",
							"description": "A clear, self-contained description of the task for the worker."
						},
						"description": {
							"type": "string",
							"description": "A short label shown in the UI for this worker."
						}
					},
					"required": ["task"]
				}
			}
		}
	}`)
}

func (t *SubAgentTool) RequiresApproval(_ json.RawMessage) bool { return false }

func (t *SubAgentTool) Execute(ctx context.Context, params json.RawMessage) (*tool.Result, error) {
	var p subAgentParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	if t.runFn == nil {
		return &tool.Result{Error: "sub-agent execution not available"}, nil
	}

	if len(p.Tasks) > 0 {
		type itemResult struct {
			index       int
			description string
			content     string
			err         error
		}
		results := make([]itemResult, len(p.Tasks))
		var wg sync.WaitGroup
		for i, task := range p.Tasks {
			if strings.TrimSpace(task.Task) == "" {
				results[i] = itemResult{index: i, description: task.Description, err: fmt.Errorf("task is required")}
				continue
			}
			wg.Add(1)
			go func(i int, task subAgentTaskParam) {
				defer wg.Done()
				content, err := t.runFn(ctx, task.Task, task.Description)
				results[i] = itemResult{
					index:       i,
					description: task.Description,
					content:     content,
					err:         err,
				}
			}(i, task)
		}
		wg.Wait()

		var b strings.Builder
		for i, result := range results {
			label := strings.TrimSpace(result.description)
			if label == "" {
				label = fmt.Sprintf("Task %d", i+1)
			}
			b.WriteString("## ")
			b.WriteString(label)
			b.WriteString("\n")
			if result.err != nil {
				b.WriteString("Error: ")
				b.WriteString(result.err.Error())
			} else {
				b.WriteString(result.content)
			}
			if i < len(results)-1 {
				b.WriteString("\n\n")
			}
		}
		return &tool.Result{Content: b.String()}, nil
	}

	if strings.TrimSpace(p.Task) == "" {
		return &tool.Result{Error: "task is required"}, nil
	}

	response, err := t.runFn(ctx, p.Task, p.Description)
	if err != nil {
		return &tool.Result{
			Error: fmt.Sprintf("sub-agent error: %v", err),
		}, nil
	}

	return &tool.Result{
		Content: response,
	}, nil
}
