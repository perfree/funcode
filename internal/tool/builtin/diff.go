package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/perfree/funcode/internal/tool"
)

type DiffTool struct{}

type diffParams struct {
	FilePath     string `json:"file_path,omitempty"`
	Staged       bool   `json:"staged,omitempty"`
	BaseRef      string `json:"base_ref,omitempty"`
	ContextLines int    `json:"context_lines,omitempty"`
}

func NewDiffTool() *DiffTool { return &DiffTool{} }

func (t *DiffTool) Name() string { return "Diff" }
func (t *DiffTool) Description() string {
	return "Show git diff for the current working tree or a specific file. Use to inspect uncommitted changes before or after edits."
}

func (t *DiffTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"file_path": {
				"type": "string",
				"description": "Optional file path to diff"
			},
			"staged": {
				"type": "boolean",
				"description": "Whether to diff staged changes"
			},
			"base_ref": {
				"type": "string",
				"description": "Optional git ref to diff against"
			},
			"context_lines": {
				"type": "integer",
				"description": "Number of context lines to show (default: 3)"
			}
		}
	}`)
}

func (t *DiffTool) RequiresApproval(_ json.RawMessage) bool { return false }

func (t *DiffTool) Execute(ctx context.Context, params json.RawMessage) (*tool.Result, error) {
	var p diffParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	cwd, _ := os.Getwd()
	contextLines := p.ContextLines
	if contextLines < 0 {
		contextLines = 0
	}
	if contextLines == 0 {
		contextLines = 3
	}

	args := []string{"diff", "--no-ext-diff", "--unified=" + strconv.Itoa(contextLines)}
	if p.Staged {
		args = append(args, "--cached")
	}
	if p.BaseRef != "" {
		args = append(args, p.BaseRef)
	}
	if p.FilePath != "" {
		path := p.FilePath
		if filepath.IsAbs(path) {
			if rel, err := filepath.Rel(cwd, path); err == nil {
				path = rel
			}
		}
		args = append(args, "--", path)
	}

	result, err := runCommandWithInput(ctx, cwd, "git", args, "", 120000)
	if result.TimedOut {
		return &tool.Result{Error: "diff timed out"}, nil
	}
	if err != nil && result.Combined == "" {
		return &tool.Result{Error: fmt.Sprintf("git diff failed: %v", err)}, nil
	}
	if result.Combined == "" {
		return &tool.Result{Content: "(no diff)"}, nil
	}
	if err != nil {
		return &tool.Result{Content: result.Combined, Error: fmt.Sprintf("git diff failed: %v", err)}, nil
	}
	return &tool.Result{Content: result.Combined}, nil
}
