package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/perfree/funcode/internal/llm"
	"github.com/perfree/funcode/pkg/types"
)

const defaultBaseURL = "http://localhost:11434"

type Provider struct {
	client  *http.Client
	name    string
	model   string
	baseURL string
}

func NewProvider(cfg *llm.ProviderConfig) (llm.Provider, error) {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	name := "ollama"
	if cfg.Name != "" {
		name = cfg.Name
	}

	return &Provider{
		client:  &http.Client{},
		name:    name,
		model:   cfg.Model,
		baseURL: baseURL,
	}, nil
}

func (p *Provider) Name() string         { return p.name }
func (p *Provider) SupportsTools() bool  { return true }
func (p *Provider) SupportsVision() bool { return true }
func (p *Provider) Capabilities() llm.ProviderCapabilities {
	return llm.DefaultCapabilities()
}

func (p *Provider) Chat(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	model := req.Model
	if model == "" {
		model = p.model
	}
	if model == "" {
		return nil, fmt.Errorf("ollama model is required")
	}

	payload := p.buildRequest(model, req)
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal ollama request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create ollama request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama chat: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read ollama response: %w", err)
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ollama chat failed: %s", strings.TrimSpace(string(respBody)))
	}

	var result ollamaResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("decode ollama response: %w", err)
	}

	return &llm.ChatResponse{
		Message: convertOllamaMessage(result.Message),
		Usage: types.Usage{
			PromptTokens:     result.PromptEvalCount,
			CompletionTokens: result.EvalCount,
			TotalTokens:      result.PromptEvalCount + result.EvalCount,
		},
	}, nil
}

func (p *Provider) ChatStream(ctx context.Context, req *llm.ChatRequest) (llm.Stream, error) {
	model := req.Model
	if model == "" {
		model = p.model
	}
	if model == "" {
		return nil, fmt.Errorf("ollama model is required")
	}

	payload := p.buildRequest(model, req)
	payload.Stream = true

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal ollama stream request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create ollama stream request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama stream: %w", err)
	}
	if resp.StatusCode >= 300 {
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama stream failed: %s", strings.TrimSpace(string(respBody)))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	return &ollamaStream{
		resp:    resp,
		scanner: scanner,
	}, nil
}

type ollamaRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Tools    []ollamaTool    `json:"tools,omitempty"`
	Stream   bool            `json:"stream"`
	Options  map[string]any  `json:"options,omitempty"`
}

type ollamaMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content,omitempty"`
	Images    []string         `json:"images,omitempty"`
	ToolCalls []ollamaToolCall `json:"tool_calls,omitempty"`
}

type ollamaTool struct {
	Type     string               `json:"type"`
	Function ollamaToolDefinition `json:"function"`
}

type ollamaToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
}

type ollamaToolCall struct {
	Function ollamaToolFunction `json:"function"`
}

type ollamaToolFunction struct {
	Name      string `json:"name"`
	Arguments any    `json:"arguments"`
}

type ollamaResponse struct {
	Message         ollamaMessage `json:"message"`
	Done            bool          `json:"done"`
	PromptEvalCount int           `json:"prompt_eval_count"`
	EvalCount       int           `json:"eval_count"`
}

type ollamaStream struct {
	resp    *http.Response
	scanner *bufio.Scanner
	pending []types.StreamEvent
	usage   *types.Usage
}

func (s *ollamaStream) Next() (types.StreamEvent, error) {
	if len(s.pending) > 0 {
		evt := s.pending[0]
		s.pending = s.pending[1:]
		return evt, nil
	}

	for s.scanner.Scan() {
		line := strings.TrimSpace(s.scanner.Text())
		if line == "" {
			continue
		}

		var chunk ollamaResponse
		if err := json.Unmarshal([]byte(line), &chunk); err != nil {
			return types.StreamEvent{}, fmt.Errorf("decode ollama stream chunk: %w", err)
		}

		if chunk.PromptEvalCount > 0 || chunk.EvalCount > 0 {
			s.usage = &types.Usage{
				PromptTokens:     chunk.PromptEvalCount,
				CompletionTokens: chunk.EvalCount,
				TotalTokens:      chunk.PromptEvalCount + chunk.EvalCount,
			}
		}

		if chunk.Message.Content != "" {
			return types.StreamEvent{Type: types.EventTextDelta, Content: chunk.Message.Content, Usage: s.usage}, nil
		}

		if len(chunk.Message.ToolCalls) > 0 {
			for i, call := range chunk.Message.ToolCalls {
				params := "{}"
				if call.Function.Arguments != nil {
					if b, err := json.Marshal(call.Function.Arguments); err == nil {
						params = string(b)
					}
				}
				tc := types.ToolCall{ID: fmt.Sprintf("tool_%d", i+1), Name: call.Function.Name, Params: params}
				s.pending = append(s.pending,
					types.StreamEvent{Type: types.EventToolCallStart, ToolCall: &tc},
					types.StreamEvent{Type: types.EventToolCallEnd, ToolCall: &tc},
				)
			}
			if len(s.pending) > 0 {
				evt := s.pending[0]
				s.pending = s.pending[1:]
				return evt, nil
			}
		}

		if chunk.Done {
			return types.StreamEvent{Type: types.EventDone, Usage: s.usage}, nil
		}
	}

	if err := s.scanner.Err(); err != nil {
		return types.StreamEvent{}, err
	}
	return types.StreamEvent{Type: types.EventDone, Usage: s.usage}, io.EOF
}

func (s *ollamaStream) Close() error {
	if s.resp != nil && s.resp.Body != nil {
		return s.resp.Body.Close()
	}
	return nil
}

func (p *Provider) buildRequest(model string, req *llm.ChatRequest) ollamaRequest {
	r := ollamaRequest{
		Model:    model,
		Messages: convertMessages(req.Messages, req.System),
		Tools:    convertTools(llm.AdaptToolDefs(p.Capabilities(), req.Tools)),
		Stream:   false,
	}

	options := make(map[string]any)
	if req.Temperature > 0 {
		options["temperature"] = req.Temperature
	}
	if req.TopP > 0 {
		options["top_p"] = req.TopP
	}
	if req.MaxTokens > 0 {
		options["num_predict"] = req.MaxTokens
	}
	if len(req.Stop) > 0 {
		options["stop"] = req.Stop
	}
	if len(options) > 0 {
		r.Options = options
	}

	return r
}

func convertMessages(msgs []types.Message, system string) []ollamaMessage {
	var result []ollamaMessage
	if system != "" {
		result = append(result, ollamaMessage{Role: "system", Content: system})
	}

	for _, msg := range msgs {
		switch msg.Role {
		case types.RoleSystem:
			if text := msg.GetText(); text != "" {
				result = append(result, ollamaMessage{Role: "system", Content: text})
			}
		case types.RoleUser, types.RoleAssistant:
			om := ollamaMessage{Role: string(msg.Role)}
			for _, block := range msg.Content {
				switch block.Type {
				case types.ContentTypeText:
					om.Content += block.Text
				case types.ContentTypeImage:
					if block.Image != nil && block.Image.Base64 != "" {
						om.Images = append(om.Images, block.Image.Base64)
					}
				case types.ContentTypeToolCall:
					if block.ToolCall != nil {
						var args any
						if block.ToolCall.Params != "" {
							_ = json.Unmarshal([]byte(block.ToolCall.Params), &args)
						}
						om.ToolCalls = append(om.ToolCalls, ollamaToolCall{Function: ollamaToolFunction{Name: block.ToolCall.Name, Arguments: args}})
					}
				}
			}
			result = append(result, om)
		case types.RoleTool:
			for _, block := range msg.Content {
				if block.Type == types.ContentTypeToolResult && block.ToolResult != nil {
					content := block.ToolResult.Content
					if block.ToolResult.Error != "" {
						content = "Error: " + block.ToolResult.Error
					}
					result = append(result, ollamaMessage{Role: "tool", Content: content})
				}
			}
		}
	}

	return result
}

func convertTools(tools []types.ToolDef) []ollamaTool {
	result := make([]ollamaTool, 0, len(tools))
	for _, t := range tools {
		params := t.Parameters
		if len(params) == 0 {
			params = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		result = append(result, ollamaTool{Type: "function", Function: ollamaToolDefinition{Name: t.Name, Description: t.Description, Parameters: params}})
	}
	return result
}

func convertOllamaMessage(msg ollamaMessage) types.Message {
	var content []types.ContentBlock
	if msg.Content != "" {
		content = append(content, types.ContentBlock{Type: types.ContentTypeText, Text: msg.Content})
	}
	for i, tc := range msg.ToolCalls {
		params := "{}"
		if tc.Function.Arguments != nil {
			if b, err := json.Marshal(tc.Function.Arguments); err == nil {
				params = string(b)
			}
		}
		content = append(content, types.ContentBlock{Type: types.ContentTypeToolCall, ToolCall: &types.ToolCall{ID: fmt.Sprintf("tool_%d", i+1), Name: tc.Function.Name, Params: params}})
	}
	return types.Message{Role: types.RoleAssistant, Content: content}
}

func init() {
	llm.DefaultRegistry.Register("ollama", NewProvider)
}
