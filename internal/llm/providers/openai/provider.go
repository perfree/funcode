package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"github.com/perfree/funcode/internal/llm"
	"github.com/perfree/funcode/pkg/types"
	openailib "github.com/sashabaranov/go-openai"
)

// Provider implements llm.Provider for OpenAI and compatible APIs
type Provider struct {
	client       *openailib.Client
	name         string
	model        string
	capabilities llm.ProviderCapabilities
}

// NewProvider creates an OpenAI provider
func NewProvider(cfg *llm.ProviderConfig) (llm.Provider, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("OpenAI API key is required")
	}

	config := openailib.DefaultConfig(cfg.APIKey)
	if cfg.BaseURL != "" {
		config.BaseURL = cfg.BaseURL
	}

	name := "openai"
	if cfg.Name != "" {
		name = cfg.Name
	}

	return &Provider{
		client:       openailib.NewClientWithConfig(config),
		name:         name,
		model:        cfg.Model,
		capabilities: llm.DefaultCapabilities(),
	}, nil
}

// NewCompatibleProvider creates a provider for OpenAI-compatible APIs (DeepSeek, Groq, etc.)
func NewCompatibleProvider(cfg *llm.ProviderConfig) (llm.Provider, error) {
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("base_url is required for OpenAI-compatible provider")
	}
	provider, err := NewProvider(cfg)
	if err != nil {
		return nil, err
	}
	if typed, ok := provider.(*Provider); ok {
		typed.capabilities = llm.OpenAICompatibleCapabilities()
	}
	return provider, nil
}

func (p *Provider) Name() string         { return p.name }
func (p *Provider) SupportsTools() bool  { return true }
func (p *Provider) SupportsVision() bool { return true }
func (p *Provider) Capabilities() llm.ProviderCapabilities {
	return p.capabilities
}

// Chat performs a non-streaming chat
func (p *Provider) Chat(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	openaiReq := p.buildRequest(req)
	resp, err := p.client.CreateChatCompletion(ctx, openaiReq)
	if err != nil {
		return nil, fmt.Errorf("openai chat: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("openai: no choices returned")
	}

	msg := p.convertResponse(resp.Choices[0].Message)
	return &llm.ChatResponse{
		Message: msg,
		Usage: types.Usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		},
	}, nil
}

// ChatStream performs a streaming chat
func (p *Provider) ChatStream(ctx context.Context, req *llm.ChatRequest) (llm.Stream, error) {
	openaiReq := p.buildRequest(req)
	openaiReq.Stream = true

	stream, err := p.client.CreateChatCompletionStream(ctx, openaiReq)
	if err != nil {
		return nil, fmt.Errorf("openai stream: %w", err)
	}

	return &openaiStream{stream: stream}, nil
}

func (p *Provider) buildRequest(req *llm.ChatRequest) openailib.ChatCompletionRequest {
	model := req.Model
	if model == "" {
		model = p.model
	}

	msgs := convertMessages(req.Messages, req.System)
	tools := convertTools(llm.AdaptToolDefs(p.Capabilities(), req.Tools))

	r := openailib.ChatCompletionRequest{
		Model:    model,
		Messages: msgs,
	}

	if len(tools) > 0 {
		r.Tools = tools
	}

	if req.MaxTokens > 0 {
		r.MaxTokens = req.MaxTokens
	}

	return r
}

func convertMessages(msgs []types.Message, system string) []openailib.ChatCompletionMessage {
	var result []openailib.ChatCompletionMessage

	if system != "" {
		result = append(result, openailib.ChatCompletionMessage{
			Role:    openailib.ChatMessageRoleSystem,
			Content: system,
		})
	}

	for _, msg := range msgs {
		switch msg.Role {
		case types.RoleUser:
			parts := convertContentParts(msg.Content)
			result = append(result, openailib.ChatCompletionMessage{
				Role:         openailib.ChatMessageRoleUser,
				MultiContent: parts,
			})
		case types.RoleAssistant:
			m := openailib.ChatCompletionMessage{
				Role: openailib.ChatMessageRoleAssistant,
			}
			// Extract text content
			for _, block := range msg.Content {
				if block.Type == types.ContentTypeText {
					m.Content = block.Text
				}
			}
			// Extract tool calls
			for _, block := range msg.Content {
				if block.Type == types.ContentTypeToolCall && block.ToolCall != nil {
					m.ToolCalls = append(m.ToolCalls, openailib.ToolCall{
						ID:   block.ToolCall.ID,
						Type: openailib.ToolTypeFunction,
						Function: openailib.FunctionCall{
							Name:      block.ToolCall.Name,
							Arguments: block.ToolCall.Params,
						},
					})
				}
			}
			result = append(result, m)
		case types.RoleTool:
			for _, block := range msg.Content {
				if block.Type == types.ContentTypeToolResult && block.ToolResult != nil {
					content := block.ToolResult.Content
					if block.ToolResult.Error != "" {
						content = "Error: " + block.ToolResult.Error
					}
					result = append(result, openailib.ChatCompletionMessage{
						Role:       openailib.ChatMessageRoleTool,
						Content:    content,
						ToolCallID: block.ToolResult.CallID,
					})
				}
			}
		case types.RoleSystem:
			result = append(result, openailib.ChatCompletionMessage{
				Role:    openailib.ChatMessageRoleSystem,
				Content: msg.GetText(),
			})
		}
	}

	return result
}

func convertContentParts(blocks []types.ContentBlock) []openailib.ChatMessagePart {
	var parts []openailib.ChatMessagePart
	for _, block := range blocks {
		switch block.Type {
		case types.ContentTypeText:
			parts = append(parts, openailib.ChatMessagePart{
				Type: openailib.ChatMessagePartTypeText,
				Text: block.Text,
			})
		case types.ContentTypeImage:
			if block.Image != nil {
				url := block.Image.URL
				if url == "" && block.Image.Base64 != "" {
					url = fmt.Sprintf("data:%s;base64,%s", block.Image.MediaType, block.Image.Base64)
				}
				parts = append(parts, openailib.ChatMessagePart{
					Type: openailib.ChatMessagePartTypeImageURL,
					ImageURL: &openailib.ChatMessageImageURL{
						URL: url,
					},
				})
			}
		}
	}
	return parts
}

func convertTools(tools []types.ToolDef) []openailib.Tool {
	var result []openailib.Tool
	for _, t := range tools {
		var params json.RawMessage
		if len(t.Parameters) > 0 {
			params = t.Parameters
		} else {
			params = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		result = append(result, openailib.Tool{
			Type: openailib.ToolTypeFunction,
			Function: &openailib.FunctionDefinition{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  params,
			},
		})
	}
	return result
}

func (p *Provider) convertResponse(msg openailib.ChatCompletionMessage) types.Message {
	var content []types.ContentBlock

	if msg.Content != "" {
		content = append(content, types.ContentBlock{
			Type: types.ContentTypeText,
			Text: msg.Content,
		})
	}

	for _, tc := range msg.ToolCalls {
		content = append(content, types.ContentBlock{
			Type: types.ContentTypeToolCall,
			ToolCall: &types.ToolCall{
				ID:     tc.ID,
				Name:   tc.Function.Name,
				Params: tc.Function.Arguments,
			},
		})
	}

	return types.Message{
		Role:    types.RoleAssistant,
		Content: content,
	}
}

// openaiStream wraps the OpenAI stream
type openaiStream struct {
	stream    *openailib.ChatCompletionStream
	pending   []types.StreamEvent
	toolCalls map[int]*types.ToolCall
	usage     *types.Usage
}

func (s *openaiStream) Next() (types.StreamEvent, error) {
	if len(s.pending) > 0 {
		evt := s.pending[0]
		s.pending = s.pending[1:]
		return evt, nil
	}

	resp, err := s.stream.Recv()
	if err != nil {
		if err == io.EOF {
			return types.StreamEvent{Type: types.EventDone, Usage: s.usage}, io.EOF
		}
		return types.StreamEvent{}, err
	}

	if len(resp.Choices) == 0 {
		return types.StreamEvent{Type: types.EventTextDelta, Usage: s.usage}, nil
	}

	delta := resp.Choices[0].Delta

	if resp.Usage != nil {
		s.usage = &types.Usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		}
	}

	// Handle tool calls
	if len(delta.ToolCalls) > 0 {
		s.collectToolCalls(delta.ToolCalls)
	}

	// Handle text content
	if delta.Content != "" {
		return types.StreamEvent{
			Type:    types.EventTextDelta,
			Content: delta.Content,
			Usage:   s.usage,
		}, nil
	}

	// Check for finish
	if resp.Choices[0].FinishReason != "" {
		s.flushToolCalls()
		if len(s.pending) > 0 {
			evt := s.pending[0]
			s.pending = s.pending[1:]
			return evt, nil
		}
		return types.StreamEvent{Type: types.EventDone, Usage: s.usage}, nil
	}

	if len(s.pending) > 0 {
		evt := s.pending[0]
		s.pending = s.pending[1:]
		return evt, nil
	}

	return types.StreamEvent{Type: types.EventTextDelta, Usage: s.usage}, nil
}

func (s *openaiStream) collectToolCalls(toolCalls []openailib.ToolCall) {
	if len(toolCalls) == 0 {
		return
	}
	if s.toolCalls == nil {
		s.toolCalls = make(map[int]*types.ToolCall)
	}
	for _, tc := range toolCalls {
		idx := 0
		if tc.Index != nil {
			idx = *tc.Index
		}
		current := s.toolCalls[idx]
		if current == nil {
			current = &types.ToolCall{}
			s.toolCalls[idx] = current
		}
		if tc.ID != "" {
			current.ID = tc.ID
		}
		if tc.Function.Name != "" {
			current.Name = tc.Function.Name
		}
		if tc.Function.Arguments != "" {
			current.Params += tc.Function.Arguments
		}
	}
}

func (s *openaiStream) flushToolCalls() {
	if len(s.toolCalls) == 0 {
		return
	}
	var indexes []int
	for idx := range s.toolCalls {
		indexes = append(indexes, idx)
	}
	sort.Ints(indexes)
	for _, idx := range indexes {
		tc := s.toolCalls[idx]
		if tc == nil {
			continue
		}
		if tc.Params == "" {
			tc.Params = "{}"
		}
		s.pending = append(s.pending,
			types.StreamEvent{Type: types.EventToolCallStart, ToolCall: tc},
			types.StreamEvent{Type: types.EventToolCallEnd, ToolCall: tc},
		)
	}
	s.toolCalls = nil
}

func (s *openaiStream) Close() error {
	s.stream.Close()
	return nil
}

func init() {
	llm.DefaultRegistry.Register("openai", NewProvider)
	llm.DefaultRegistry.Register("openai-compatible", NewCompatibleProvider)
}
