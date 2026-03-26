package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/perfree/funcode/internal/tool"
)

type PatchTool struct{}

type patchParams struct {
	Patch   string `json:"patch"`
	Reverse bool   `json:"reverse,omitempty"`
	Cwd     string `json:"cwd,omitempty"`
}

func NewPatchTool() *PatchTool { return &PatchTool{} }

func (t *PatchTool) Name() string { return "Patch" }
func (t *PatchTool) Description() string {
	return "Apply a unified diff patch to the working tree. Use for multi-hunk or multi-file edits when a single Edit call would be fragile."
}

func (t *PatchTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"patch": {
				"type": "string",
				"description": "Unified diff patch content"
			},
			"reverse": {
				"type": "boolean",
				"description": "Whether to reverse the patch"
			},
			"cwd": {
				"type": "string",
				"description": "Optional working directory to apply the patch from"
			}
		},
		"required": ["patch"]
	}`)
}

func (t *PatchTool) RequiresApproval(_ json.RawMessage) bool { return true }

func (t *PatchTool) Execute(ctx context.Context, params json.RawMessage) (*tool.Result, error) {
	var p patchParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	if p.Patch == "" {
		return &tool.Result{Error: "patch is required"}, nil
	}

	cwd := p.Cwd
	if cwd == "" {
		cwd, _ = os.Getwd()
	}

	args := []string{"apply", "--whitespace=nowarn", "--recount", "--unidiff-zero", "--unsafe-paths", "-"}
	if p.Reverse {
		args = append(args[:1], append([]string{"--reverse"}, args[1:]...)...)
	}

	result, err := runCommandWithInput(ctx, cwd, "git", args, p.Patch, 120000)
	if result.TimedOut {
		return &tool.Result{Error: "patch timed out"}, nil
	}
	if err != nil {
		if result.Combined != "" {
			return &tool.Result{
				Content: result.Combined,
				Error:   fmt.Sprintf("patch apply failed: %v", err),
			}, nil
		}
		return &tool.Result{Error: fmt.Sprintf("patch apply failed: %v", err)}, nil
	}

	content := "Patch applied successfully"
	if result.Combined != "" {
		content += "\n" + result.Combined
	}
	return &tool.Result{Content: content}, nil
}
