package builtin

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/perfree/funcode/internal/agent"
	"github.com/perfree/funcode/internal/tool"
)

type CollaborateFunc func(ctx context.Context, plan agent.CollaborationPlan) (*agent.CollaborationResult, error)

type CollaborateTool struct {
	runFn CollaborateFunc
	roles []RoleInfo
}

func NewCollaborateTool(runFn CollaborateFunc, roles []RoleInfo) *CollaborateTool {
	return &CollaborateTool{
		runFn: runFn,
		roles: roles,
	}
}

type collaborateParams struct {
	Goal    string                 `json:"goal"`
	Context string                 `json:"context"`
	Tasks   []collaborateTaskParam `json:"tasks"`
}

type collaborateTaskParam struct {
	ID        string   `json:"id"`
	Role      string   `json:"role"`
	Title     string   `json:"title"`
	Task      string   `json:"task"`
	DependsOn []string `json:"depends_on"`
}

func (t *CollaborateTool) Name() string { return "Collaborate" }

func (t *CollaborateTool) Description() string {
	desc := "Run a real multi-role collaboration workflow with explicit task splitting, dependencies, lifecycle tracking, and result aggregation.\nUse shared context only when it is concise and genuinely useful; otherwise let each role inspect what it needs.\nAvailable roles:\n"
	for _, r := range t.roles {
		desc += fmt.Sprintf("- %s: %s\n", r.Name, r.Description)
	}
	return desc
}

func (t *CollaborateTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"goal": {
				"type": "string",
				"description": "Overall collaboration goal"
			},
			"context": {
				"type": "string",
				"description": "Optional known project context, summary, constraints, or findings shared with all participating roles"
			},
			"tasks": {
				"type": "array",
				"description": "Explicit subtasks for different roles",
				"items": {
					"type": "object",
					"properties": {
						"id": {"type": "string"},
						"role": {"type": "string"},
						"title": {"type": "string"},
						"task": {"type": "string"},
						"depends_on": {
							"type": "array",
							"items": {"type": "string"}
						}
					},
					"required": ["id", "role", "task"]
				}
			}
		},
		"required": ["goal", "tasks"]
	}`)
}

func (t *CollaborateTool) RequiresApproval(_ json.RawMessage) bool { return false }

func (t *CollaborateTool) Execute(ctx context.Context, params json.RawMessage) (*tool.Result, error) {
	var p collaborateParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	if p.Goal == "" || len(p.Tasks) == 0 {
		return &tool.Result{Error: "goal and tasks are required"}, nil
	}
	if t.runFn == nil {
		return &tool.Result{Error: "collaboration not available"}, nil
	}

	plan := agent.CollaborationPlan{Goal: p.Goal, SharedContext: p.Context}
	for _, item := range p.Tasks {
		plan.Tasks = append(plan.Tasks, agent.CollaborationTask{
			ID:        item.ID,
			Role:      item.Role,
			Title:     item.Title,
			Task:      item.Task,
			DependsOn: item.DependsOn,
			Status:    agent.CollaborationTaskPending,
		})
	}

	result, err := t.runFn(ctx, plan)
	if err != nil {
		return &tool.Result{Error: err.Error()}, nil
	}
	return &tool.Result{Content: result.Summary}, nil
}
