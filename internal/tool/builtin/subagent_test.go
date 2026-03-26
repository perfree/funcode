package builtin

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestSubAgentToolExecuteBatchAggregatesWorkerResults(t *testing.T) {
	callCount := 0
	tool := NewSubAgentTool(func(ctx context.Context, task string, description string) (string, error) {
		callCount++
		if description == "" {
			description = "worker"
		}
		return "done: " + description + " -> " + task, nil
	})

	params := json.RawMessage(`{
		"tasks": [
			{"description": "Inspect README", "task": "Read and summarize README.md"},
			{"description": "Inspect go.mod", "task": "Read and summarize go.mod"}
		]
	}`)

	result, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("execute batch agent tool: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("unexpected tool error: %s", result.Error)
	}
	if callCount != 2 {
		t.Fatalf("expected runFn to be called twice, got %d", callCount)
	}
	if !strings.Contains(result.Content, "## Inspect README") {
		t.Fatalf("expected README section in result, got %s", result.Content)
	}
	if !strings.Contains(result.Content, "## Inspect go.mod") {
		t.Fatalf("expected go.mod section in result, got %s", result.Content)
	}
}
