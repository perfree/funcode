package tool

import (
	"context"
	"encoding/json"

	"github.com/perfree/funcode/pkg/types"
)

// Tool is the interface for all tools
type Tool interface {
	Name() string
	Description() string
	Schema() json.RawMessage // JSON Schema for parameters
	Execute(ctx context.Context, params json.RawMessage) (*Result, error)
	RequiresApproval(params json.RawMessage) bool
}

// Result is the tool execution result
type Result struct {
	Content  string         `json:"content"`
	Error    string         `json:"error,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// ToToolDef converts a Tool to a ToolDef for LLM
func ToToolDef(t Tool) types.ToolDef {
	return types.ToolDef{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters:  t.Schema(),
	}
}

// ToToolDefs converts a slice of Tools to ToolDefs
func ToToolDefs(tools []Tool) []types.ToolDef {
	defs := make([]types.ToolDef, len(tools))
	for i, t := range tools {
		defs[i] = ToToolDef(t)
	}
	return defs
}
