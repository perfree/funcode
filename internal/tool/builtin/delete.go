package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/perfree/funcode/internal/tool"
)

type DeleteTool struct{}

type deleteParams struct {
	FilePath string `json:"file_path"`
}

func NewDeleteTool() *DeleteTool { return &DeleteTool{} }

func (t *DeleteTool) Name() string { return "Delete" }
func (t *DeleteTool) Description() string {
	return "Delete a file or empty directory at any absolute path. IMPORTANT: Always use Glob or Read first to verify the exact file path before deleting. Never guess file paths."
}

func (t *DeleteTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"file_path": {
				"type": "string",
				"description": "The absolute path to the file or empty directory to delete"
			}
		},
		"required": ["file_path"]
	}`)
}

func (t *DeleteTool) RequiresApproval(_ json.RawMessage) bool { return true }

func (t *DeleteTool) Execute(ctx context.Context, params json.RawMessage) (*tool.Result, error) {
	var p deleteParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	info, err := os.Stat(p.FilePath)
	if err != nil {
		return &tool.Result{Error: fmt.Sprintf("file not found: %v", err)}, nil
	}

	if info.IsDir() {
		if err := os.Remove(p.FilePath); err != nil {
			return &tool.Result{Error: fmt.Sprintf("delete directory failed (must be empty): %v", err)}, nil
		}
		return &tool.Result{Content: fmt.Sprintf("Successfully deleted directory: %s", p.FilePath)}, nil
	}

	if err := os.Remove(p.FilePath); err != nil {
		return &tool.Result{Error: err.Error()}, nil
	}
	return &tool.Result{Content: fmt.Sprintf("Successfully deleted: %s", p.FilePath)}, nil
}
