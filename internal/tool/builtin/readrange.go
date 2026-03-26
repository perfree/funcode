package builtin

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/perfree/funcode/internal/tool"
)

type ReadRangeTool struct{}

type readRangeParams struct {
	FilePath  string `json:"file_path"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line,omitempty"`
	MaxLines  int    `json:"max_lines,omitempty"`
}

func NewReadRangeTool() *ReadRangeTool { return &ReadRangeTool{} }

func (t *ReadRangeTool) Name() string { return "ReadRange" }
func (t *ReadRangeTool) Description() string {
	return "Read a specific line range from a file. Prefer this over reading the whole file when you only need a focused snippet."
}

func (t *ReadRangeTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"file_path": {
				"type": "string",
				"description": "The file to read"
			},
			"start_line": {
				"type": "integer",
				"description": "1-based start line"
			},
			"end_line": {
				"type": "integer",
				"description": "1-based end line (inclusive)"
			},
			"max_lines": {
				"type": "integer",
				"description": "Maximum number of lines to return when end_line is omitted (default: 200)"
			}
		},
		"required": ["file_path", "start_line"]
	}`)
}

func (t *ReadRangeTool) RequiresApproval(_ json.RawMessage) bool { return false }

func (t *ReadRangeTool) Execute(ctx context.Context, params json.RawMessage) (*tool.Result, error) {
	var p readRangeParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	if p.StartLine <= 0 {
		return &tool.Result{Error: "start_line must be >= 1"}, nil
	}
	if p.EndLine > 0 && p.EndLine < p.StartLine {
		return &tool.Result{Error: "end_line must be >= start_line"}, nil
	}

	file, err := os.Open(p.FilePath)
	if err != nil {
		return &tool.Result{Error: err.Error()}, nil
	}
	defer file.Close()

	maxLines := p.MaxLines
	if maxLines <= 0 {
		maxLines = 200
	}

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	endLine := p.EndLine
	if endLine == 0 {
		endLine = p.StartLine + maxLines - 1
	}

	var lines []string
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		if lineNum < p.StartLine {
			continue
		}
		if lineNum > endLine {
			break
		}
		lines = append(lines, fmt.Sprintf("%6d\t%s", lineNum, scanner.Text()))
	}

	if err := scanner.Err(); err != nil {
		return &tool.Result{Error: fmt.Sprintf("reading file: %v", err)}, nil
	}
	if len(lines) == 0 {
		return &tool.Result{Content: "(no lines in requested range)"}, nil
	}

	return &tool.Result{Content: strings.Join(lines, "\n")}, nil
}
