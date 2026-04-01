package builtin

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/perfree/funcode/internal/tool"
)

// DelegateFunc is the function that actually runs another agent by role name
type DelegateFunc func(ctx context.Context, roleName string, task string, contextText string) (string, error)

// DelegateTool allows an agent to delegate a task to another role
type DelegateTool struct {
	delegateFn DelegateFunc
	roles      []RoleInfo
}

// RoleInfo holds display info for available roles
type RoleInfo struct {
	Name        string
	Description string
}

// NewDelegateTool creates a delegate tool
func NewDelegateTool(delegateFn DelegateFunc, roles []RoleInfo) *DelegateTool {
	return &DelegateTool{
		delegateFn: delegateFn,
		roles:      roles,
	}
}

type delegateParams struct {
	Role    string `json:"role"`
	Task    string `json:"task"`
	Reason  string `json:"reason"`
	Context string `json:"context"`
}

func (t *DelegateTool) Name() string { return "Delegate" }

func (t *DelegateTool) Description() string {
	desc := `Delegate a task to another specialist role/agent.

The delegated role will explore the codebase independently using its own tools. Your job is to give a clear, specific task — not to pre-digest the codebase for it.

Guidance:
  1. Write a specific task with clear deliverables. Say what you need, not how to do it.
  2. Include depth expectations: "read the key files", "trace the call chain", "review at least N modules".
  3. If you discovered critical constraints or decisions, pass them as brief context.
  4. Do NOT dump directory trees, large file contents, or verbose summaries. The delegate can read the code itself.
  5. For review/analysis tasks, explicitly ask for evidence-based findings with file paths and code references.

Good examples:
  - "Review the authentication module in internal/auth/. Read the key source files, trace the login flow end-to-end, and identify security risks. Cite specific code."
  - "Analyze the database layer in internal/store/. Read at least the main query files and connection management. Report performance issues with line references."

Bad examples:
  - "Review the architecture" (too vague, no depth requirement)
  - Pasting hundreds of lines of file contents when the delegate can read them directly

Available roles to delegate to:
`
	for _, r := range t.roles {
		desc += fmt.Sprintf("- %s: %s\n", r.Name, r.Description)
	}
	return desc
}

func (t *DelegateTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"role": {
				"type": "string",
				"description": "The role to delegate to"
			},
			"task": {
				"type": "string",
				"description": "A clear description of the task for the delegate. Be specific about what you need."
			},
			"reason": {
				"type": "string",
				"description": "Brief reason why you are delegating (e.g., 'Need architecture design before implementation')"
			},
			"context": {
				"type": "string",
				"description": "Optional known project context, summary, constraints, or findings to pass to the delegated role"
			}
		},
		"required": ["role", "task"]
	}`)
}

func (t *DelegateTool) RequiresApproval(_ json.RawMessage) bool { return false }

func (t *DelegateTool) Execute(ctx context.Context, params json.RawMessage) (*tool.Result, error) {
	var p delegateParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	if p.Role == "" || p.Task == "" {
		return &tool.Result{Error: "role and task are required"}, nil
	}
	if !t.hasRole(p.Role) {
		return &tool.Result{Error: fmt.Sprintf("unknown role: %s", p.Role)}, nil
	}

	if t.delegateFn == nil {
		return &tool.Result{Error: "delegation not available"}, nil
	}

	response, err := t.delegateFn(ctx, p.Role, p.Task, p.Context)
	if err != nil {
		return &tool.Result{
			Error: fmt.Sprintf("delegate to %s failed: %v", p.Role, err),
		}, nil
	}

	return &tool.Result{
		Content: fmt.Sprintf("[Response from %s]:\n%s", p.Role, response),
	}, nil
}

func (t *DelegateTool) hasRole(role string) bool {
	for _, item := range t.roles {
		if item.Name == role {
			return true
		}
	}
	return false
}
