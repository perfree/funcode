package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/perfree/funcode/internal/tool"
)

type GlobTool struct{}

type globParams struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path,omitempty"`
}

func NewGlobTool() *GlobTool { return &GlobTool{} }

func (t *GlobTool) Name() string { return "Glob" }
func (t *GlobTool) Description() string {
	return `Find files matching a glob pattern. Results sorted by modification time (newest first).

Usage:
- First choice for exploring project structure and finding files by name.
- Use '*' to list a directory, '**/*.go' to find all Go files recursively, 'src/**/*.ts' to scope to a subdirectory.
- Pass 'path' to search in a specific directory instead of the working directory.
- Prefer Glob over Bash with ls/dir/find — it's faster and cross-platform.`
}

func (t *GlobTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"pattern": {
				"type": "string",
				"description": "Glob pattern (e.g., '**/*.go', 'src/**/*.ts')"
			},
			"path": {
				"type": "string",
				"description": "Base directory to search in (default: current directory)"
			}
		},
		"required": ["pattern"]
	}`)
}

func (t *GlobTool) RequiresApproval(_ json.RawMessage) bool { return false }

func (t *GlobTool) Execute(ctx context.Context, params json.RawMessage) (*tool.Result, error) {
	var p globParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	baseDir := p.Path
	if baseDir == "" {
		baseDir, _ = os.Getwd()
	}

	var matches []string
	err := filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if info.IsDir() {
			name := info.Name()
			if name == ".git" || name == "node_modules" || name == ".idea" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}

		relPath, _ := filepath.Rel(baseDir, path)
		relPath = filepath.ToSlash(relPath)
		matched, _ := filepath.Match(p.Pattern, filepath.Base(relPath))
		if !matched {
			// Try matching full relative path for ** patterns
			matched = matchDoublestar(p.Pattern, relPath)
		}
		if matched {
			matches = append(matches, relPath)
		}
		return nil
	})

	if err != nil {
		return &tool.Result{Error: err.Error()}, nil
	}

	// Sort by modification time (newest first)
	sort.Slice(matches, func(i, j int) bool {
		iInfo, _ := os.Stat(filepath.Join(baseDir, matches[i]))
		jInfo, _ := os.Stat(filepath.Join(baseDir, matches[j]))
		if iInfo == nil || jInfo == nil {
			return false
		}
		return iInfo.ModTime().After(jInfo.ModTime())
	})

	if len(matches) == 0 {
		return &tool.Result{Content: "No files found matching pattern: " + p.Pattern}, nil
	}

	return &tool.Result{
		Content: fmt.Sprintf("Found %d files:\n%s", len(matches), strings.Join(matches, "\n")),
	}, nil
}

// matchDoublestar does simplified ** pattern matching
func matchDoublestar(pattern, path string) bool {
	// Simple implementation: strip **/ prefix and match the rest
	if strings.HasPrefix(pattern, "**/") {
		suffix := pattern[3:]
		matched, _ := filepath.Match(suffix, filepath.Base(path))
		return matched
	}
	matched, _ := filepath.Match(pattern, path)
	return matched
}
