package builtin

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/perfree/funcode/internal/tool"
)

type GrepTool struct{}

type grepParams struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path,omitempty"`
	Glob    string `json:"glob,omitempty"`
	Context int    `json:"context,omitempty"`
}

func NewGrepTool() *GrepTool { return &GrepTool{} }

func (t *GrepTool) Name() string { return "Grep" }
func (t *GrepTool) Description() string {
	return "Search file contents using regex patterns. Returns matching lines with file paths and line numbers."
}

func (t *GrepTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"pattern": {
				"type": "string",
				"description": "Regular expression pattern to search for"
			},
			"path": {
				"type": "string",
				"description": "File or directory to search in (default: current directory)"
			},
			"glob": {
				"type": "string",
				"description": "Glob pattern to filter files (e.g., '*.go')"
			},
			"context": {
				"type": "integer",
				"description": "Number of context lines before and after matches"
			}
		},
		"required": ["pattern"]
	}`)
}

func (t *GrepTool) RequiresApproval(_ json.RawMessage) bool { return false }

func (t *GrepTool) Execute(ctx context.Context, params json.RawMessage) (*tool.Result, error) {
	var p grepParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	re, err := regexp.Compile(p.Pattern)
	if err != nil {
		return &tool.Result{Error: fmt.Sprintf("invalid regex: %v", err)}, nil
	}

	searchPath := p.Path
	if searchPath == "" {
		searchPath, _ = os.Getwd()
	}

	info, err := os.Stat(searchPath)
	if err != nil {
		return &tool.Result{Error: err.Error()}, nil
	}

	var results []string
	maxResults := 100

	if !info.IsDir() {
		matches := searchFile(searchPath, re, p.Context)
		results = append(results, matches...)
	} else {
		filepath.Walk(searchPath, func(path string, fi os.FileInfo, err error) error {
			if err != nil || fi.IsDir() {
				if fi != nil && fi.IsDir() {
					name := fi.Name()
					if name == ".git" || name == "node_modules" || name == "vendor" || name == ".idea" {
						return filepath.SkipDir
					}
				}
				return nil
			}
			if len(results) >= maxResults {
				return filepath.SkipAll
			}

			// Apply glob filter
			if p.Glob != "" {
				matched, _ := filepath.Match(p.Glob, filepath.Base(path))
				if !matched {
					return nil
				}
			}

			matches := searchFile(path, re, p.Context)
			results = append(results, matches...)
			return nil
		})
	}

	if len(results) == 0 {
		return &tool.Result{Content: "No matches found for: " + p.Pattern}, nil
	}

	output := strings.Join(results, "\n")
	if len(results) >= maxResults {
		output += fmt.Sprintf("\n... (truncated, showing first %d matches)", maxResults)
	}

	return &tool.Result{Content: output}, nil
}

func searchFile(path string, re *regexp.Regexp, contextLines int) []string {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	var results []string
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if re.MatchString(line) {
			results = append(results, fmt.Sprintf("%s:%d: %s", path, lineNum, line))
		}
	}
	return results
}
