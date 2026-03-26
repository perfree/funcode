package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/perfree/funcode/internal/tool"
)

type WriteTool struct{}

type writeParams struct {
	FilePath string `json:"file_path"`
	Content  string `json:"content"`
}

func NewWriteTool() *WriteTool { return &WriteTool{} }

func (t *WriteTool) Name() string { return "Write" }
func (t *WriteTool) Description() string {
	return "Write content to a file at any absolute path. Creates the file and parent directories if they don't exist, overwrites if it does."
}

func (t *WriteTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"file_path": {
				"type": "string",
				"description": "The path to the file to write"
			},
			"content": {
				"type": "string",
				"description": "The content to write to the file"
			}
		},
		"required": ["file_path", "content"]
	}`)
}

func (t *WriteTool) RequiresApproval(_ json.RawMessage) bool { return true }

func (t *WriteTool) Execute(ctx context.Context, params json.RawMessage) (*tool.Result, error) {
	var p writeParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	// Ensure parent directory exists
	dir := filepath.Dir(p.FilePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return &tool.Result{Error: fmt.Sprintf("creating directory: %v", err)}, nil
	}

	if err := os.WriteFile(p.FilePath, []byte(p.Content), 0644); err != nil {
		return &tool.Result{Error: err.Error()}, nil
	}

	return &tool.Result{Content: fmt.Sprintf("Successfully wrote to %s", p.FilePath)}, nil
}
