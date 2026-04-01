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

type ReadTool struct{}

type readParams struct {
	FilePath string `json:"file_path"`
	Offset   int    `json:"offset,omitempty"`
	Limit    int    `json:"limit,omitempty"`
}

func NewReadTool() *ReadTool { return &ReadTool{} }

func (t *ReadTool) Name() string { return "Read" }
func (t *ReadTool) Description() string {
	return `Read a file and return its content with line numbers.

Usage:
- You MUST Read a file before editing it. Never edit a file you haven't read in this conversation.
- Use offset and limit for large files — read the relevant section instead of the whole file.
- Prefer Read over Bash with cat/type — it's faster and shows line numbers.
- If unsure whether a file exists, use Glob first to verify the path.`
}

func (t *ReadTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"file_path": {
				"type": "string",
				"description": "The path to the file to read"
			},
			"offset": {
				"type": "integer",
				"description": "Line number to start reading from (1-based)"
			},
			"limit": {
				"type": "integer",
				"description": "Maximum number of lines to read"
			}
		},
		"required": ["file_path"]
	}`)
}

func (t *ReadTool) RequiresApproval(_ json.RawMessage) bool { return false }

func (t *ReadTool) Execute(ctx context.Context, params json.RawMessage) (*tool.Result, error) {
	var p readParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	file, err := os.Open(p.FilePath)
	if err != nil {
		return &tool.Result{Error: err.Error()}, nil
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB buffer

	var lines []string
	lineNum := 0
	offset := p.Offset
	if offset <= 0 {
		offset = 1
	}
	limit := p.Limit
	if limit <= 0 {
		limit = 2000
	}

	for scanner.Scan() {
		lineNum++
		if lineNum < offset {
			continue
		}
		if len(lines) >= limit {
			break
		}
		lines = append(lines, fmt.Sprintf("%6d\t%s", lineNum, scanner.Text()))
	}

	if err := scanner.Err(); err != nil {
		return &tool.Result{Error: fmt.Sprintf("reading file: %v", err)}, nil
	}

	if len(lines) == 0 {
		return &tool.Result{Content: "(empty file)"}, nil
	}

	return &tool.Result{Content: strings.Join(lines, "\n")}, nil
}
