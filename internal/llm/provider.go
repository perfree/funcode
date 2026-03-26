package llm

import (
	"context"

	"github.com/perfree/funcode/pkg/types"
)

// Provider is the unified interface for all LLM providers
type Provider interface {
	Name() string
	Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
	ChatStream(ctx context.Context, req *ChatRequest) (Stream, error)
	SupportsTools() bool
	SupportsVision() bool
	Capabilities() ProviderCapabilities
}

// ChatRequest is the unified chat request
type ChatRequest struct {
	Model       string
	Messages    []types.Message
	Tools       []types.ToolDef
	System      string
	MaxTokens   int
	Temperature float64
	TopP        float64
	Stop        []string
}

// ChatResponse is the unified chat response
type ChatResponse struct {
	Message types.Message
	Usage   types.Usage
}

// Stream is the streaming response interface
type Stream interface {
	Next() (types.StreamEvent, error)
	Close() error
}
