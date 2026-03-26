package openai

import (
	"testing"

	"github.com/perfree/funcode/pkg/types"
	openailib "github.com/sashabaranov/go-openai"
)

func TestOpenAIStreamCollectAndFlushMultipleToolCalls(t *testing.T) {
	s := &openaiStream{}
	idx0 := 0
	idx1 := 1

	s.collectToolCalls([]openailib.ToolCall{
		{
			Index: &idx1,
			ID:    "call_b",
			Function: openailib.FunctionCall{
				Name:      "Read",
				Arguments: `{"file":"README`,
			},
		},
		{
			Index: &idx0,
			ID:    "call_a",
			Function: openailib.FunctionCall{
				Name:      "Glob",
				Arguments: `{"pattern":"go`,
			},
		},
	})
	s.collectToolCalls([]openailib.ToolCall{
		{
			Index: &idx0,
			Function: openailib.FunctionCall{
				Arguments: `.mod"}`,
			},
		},
		{
			Index: &idx1,
			Function: openailib.FunctionCall{
				Arguments: `.md"}`,
			},
		},
	})

	s.flushToolCalls()

	if len(s.pending) != 4 {
		t.Fatalf("expected 4 pending events, got %d", len(s.pending))
	}

	if s.pending[0].Type != types.EventToolCallStart || s.pending[0].ToolCall == nil || s.pending[0].ToolCall.ID != "call_a" {
		t.Fatalf("expected first event to be start for call_a, got %#v", s.pending[0])
	}
	if s.pending[1].Type != types.EventToolCallEnd || s.pending[1].ToolCall == nil || s.pending[1].ToolCall.Params != `{"pattern":"go.mod"}` {
		t.Fatalf("expected second event to end call_a with merged params, got %#v", s.pending[1])
	}
	if s.pending[2].Type != types.EventToolCallStart || s.pending[2].ToolCall == nil || s.pending[2].ToolCall.ID != "call_b" {
		t.Fatalf("expected third event to be start for call_b, got %#v", s.pending[2])
	}
	if s.pending[3].Type != types.EventToolCallEnd || s.pending[3].ToolCall == nil || s.pending[3].ToolCall.Params != `{"file":"README.md"}` {
		t.Fatalf("expected fourth event to end call_b with merged params, got %#v", s.pending[3])
	}
}
