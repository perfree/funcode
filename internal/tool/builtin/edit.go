package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/perfree/funcode/internal/tool"
)

type EditTool struct{}

type editParams struct {
	FilePath   string `json:"file_path"`
	OldString  string `json:"old_string,omitempty"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all,omitempty"`
	LineStart  int    `json:"line_start,omitempty"`
	LineEnd    int    `json:"line_end,omitempty"`
}

func NewEditTool() *EditTool { return &EditTool{} }

func (t *EditTool) Name() string { return "Edit" }
func (t *EditTool) Description() string {
	return `Edit an existing file. You MUST Read the file first before using this tool.

Two modes:
1. String mode: provide old_string + new_string to replace an exact text match. old_string must be unique in the file (or set replace_all=true). Include enough surrounding context in old_string to make it unique.
2. Line mode: provide line_start + line_end + new_string to replace a range of lines (1-based, inclusive).

Prefer Edit over Write for modifying existing files — it's safer and shows exactly what changed.`
}

func (t *EditTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"file_path": {
				"type": "string",
				"description": "The path to the file to edit"
			},
			"old_string": {
				"type": "string",
				"description": "String mode: the exact string to find and replace"
			},
			"new_string": {
				"type": "string",
				"description": "The replacement content"
			},
			"replace_all": {
				"type": "boolean",
				"description": "String mode: replace all occurrences (default: false)"
			},
			"line_start": {
				"type": "integer",
				"description": "Line mode: start line number (1-based, inclusive)"
			},
			"line_end": {
				"type": "integer",
				"description": "Line mode: end line number (1-based, inclusive)"
			}
		},
		"required": ["file_path", "new_string"]
	}`)
}

func (t *EditTool) RequiresApproval(_ json.RawMessage) bool { return true }

func (t *EditTool) Execute(ctx context.Context, params json.RawMessage) (*tool.Result, error) {
	var p editParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	// Line mode: replace lines by number range
	if p.LineStart > 0 && p.LineEnd > 0 {
		return t.executeLineMode(p)
	}

	// String mode: replace by exact match
	if p.OldString == "" {
		return &tool.Result{Error: "either old_string or line_start/line_end is required"}, nil
	}
	return t.executeStringMode(p)
}

func (t *EditTool) executeLineMode(p editParams) (*tool.Result, error) {
	if p.LineEnd < p.LineStart {
		return &tool.Result{Error: fmt.Sprintf("line_end (%d) must be >= line_start (%d)", p.LineEnd, p.LineStart)}, nil
	}

	content, err := os.ReadFile(p.FilePath)
	if err != nil {
		return &tool.Result{Error: err.Error()}, nil
	}

	lines := strings.Split(string(content), "\n")
	totalLines := len(lines)

	if p.LineStart < 1 || p.LineStart > totalLines {
		return &tool.Result{Error: fmt.Sprintf("line_start %d out of range (file has %d lines)", p.LineStart, totalLines)}, nil
	}
	if p.LineEnd > totalLines {
		p.LineEnd = totalLines
	}

	// Build new content: lines before + new_string + lines after
	var result []string
	result = append(result, lines[:p.LineStart-1]...)

	// Insert new content (may be multiple lines)
	if p.NewString != "" {
		newLines := strings.Split(p.NewString, "\n")
		result = append(result, newLines...)
	}

	if p.LineEnd < totalLines {
		result = append(result, lines[p.LineEnd:]...)
	}

	newContent := strings.Join(result, "\n")
	if err := os.WriteFile(p.FilePath, []byte(newContent), 0644); err != nil {
		return &tool.Result{Error: err.Error()}, nil
	}

	replacedCount := p.LineEnd - p.LineStart + 1
	return &tool.Result{
		Content: fmt.Sprintf("Successfully edited %s (replaced lines %d-%d, %d lines)", p.FilePath, p.LineStart, p.LineEnd, replacedCount),
	}, nil
}

func (t *EditTool) executeStringMode(p editParams) (*tool.Result, error) {
	content, err := os.ReadFile(p.FilePath)
	if err != nil {
		return &tool.Result{Error: err.Error()}, nil
	}

	fileContent := string(content)
	count := strings.Count(fileContent, p.OldString)

	if count == 0 {
		return &tool.Result{Error: "old_string not found in file"}, nil
	}

	if count > 1 && !p.ReplaceAll {
		return &tool.Result{
			Error: fmt.Sprintf("old_string found %d times, set replace_all=true or provide more context", count),
		}, nil
	}

	var newContent string
	if p.ReplaceAll {
		newContent = strings.ReplaceAll(fileContent, p.OldString, p.NewString)
	} else {
		newContent = strings.Replace(fileContent, p.OldString, p.NewString, 1)
	}

	if err := os.WriteFile(p.FilePath, []byte(newContent), 0644); err != nil {
		return &tool.Result{Error: err.Error()}, nil
	}

	return &tool.Result{
		Content: fmt.Sprintf("Successfully edited %s (%d replacement(s))", p.FilePath, count),
	}, nil
}
