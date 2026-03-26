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

type TreeTool struct{}

type treeParams struct {
	Path       string `json:"path,omitempty"`
	MaxDepth   int    `json:"max_depth,omitempty"`
	MaxEntries int    `json:"max_entries,omitempty"`
	DirsOnly   bool   `json:"dirs_only,omitempty"`
	ShowHidden bool   `json:"show_hidden,omitempty"`
}

func NewTreeTool() *TreeTool { return &TreeTool{} }

func (t *TreeTool) Name() string { return "Tree" }
func (t *TreeTool) Description() string {
	return "Display a directory tree. Use to quickly understand the project structure before opening files."
}

func (t *TreeTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Directory path to inspect (default: current working directory)"
			},
			"max_depth": {
				"type": "integer",
				"description": "Maximum depth to recurse (default: 4)"
			},
			"max_entries": {
				"type": "integer",
				"description": "Maximum number of tree entries to return (default: 200)"
			},
			"dirs_only": {
				"type": "boolean",
				"description": "Whether to show directories only"
			},
			"show_hidden": {
				"type": "boolean",
				"description": "Whether to include hidden files and directories"
			}
		}
	}`)
}

func (t *TreeTool) RequiresApproval(_ json.RawMessage) bool { return false }

func (t *TreeTool) Execute(ctx context.Context, params json.RawMessage) (*tool.Result, error) {
	var p treeParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	root := p.Path
	if root == "" {
		root, _ = os.Getwd()
	}

	info, err := os.Stat(root)
	if err != nil {
		return &tool.Result{Error: err.Error()}, nil
	}
	if !info.IsDir() {
		return &tool.Result{Error: "path is not a directory"}, nil
	}

	maxDepth := p.MaxDepth
	if maxDepth <= 0 {
		maxDepth = 4
	}
	maxEntries := p.MaxEntries
	if maxEntries <= 0 {
		maxEntries = 200
	}

	var lines []string
	lines = append(lines, filepath.Clean(root))
	count := 0
	truncated := false

	var walk func(dir string, prefix string, depth int)
	walk = func(dir string, prefix string, depth int) {
		if truncated || depth > maxDepth {
			return
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			lines = append(lines, prefix+"[error] "+err.Error())
			return
		}

		sort.Slice(entries, func(i, j int) bool {
			if entries[i].IsDir() != entries[j].IsDir() {
				return entries[i].IsDir()
			}
			return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
		})

		var filtered []os.DirEntry
		for _, entry := range entries {
			name := entry.Name()
			if !p.ShowHidden && strings.HasPrefix(name, ".") {
				continue
			}
			if isSkippedTreeDir(name) && entry.IsDir() {
				continue
			}
			if p.DirsOnly && !entry.IsDir() {
				continue
			}
			filtered = append(filtered, entry)
		}

		for i, entry := range filtered {
			if count >= maxEntries {
				truncated = true
				lines = append(lines, prefix+"... (truncated)")
				return
			}

			connector := "|- "
			nextPrefix := prefix + "|  "
			if i == len(filtered)-1 {
				connector = "\\- "
				nextPrefix = prefix + "   "
			}

			name := entry.Name()
			if entry.IsDir() {
				name += string(filepath.Separator)
			}
			lines = append(lines, prefix+connector+name)
			count++

			if entry.IsDir() {
				walk(filepath.Join(dir, entry.Name()), nextPrefix, depth+1)
			}
		}
	}

	walk(root, "", 1)

	return &tool.Result{Content: strings.Join(lines, "\n")}, nil
}

func isSkippedTreeDir(name string) bool {
	switch name {
	case ".git", ".idea", "node_modules", "vendor":
		return true
	default:
		return false
	}
}
