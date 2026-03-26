package anthropic

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

const defaultBaseURL = "https://api.anthropic.com/v1"

type Provider struct {
	client  *http.Client
	name    string
	model   string
	baseURL string
	apiKey  string
}

func NewProvider(cfg *llm.ProviderConfig) (llm.Provider, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("Anthropic API key is required")
	}

	baseURL := normalizeBaseURL(cfg.BaseURL)

	name := "anthropic"
	if cfg.Name != "" {
		name = cfg.Name
	}

	return &Provider{
		client:  &http.Client{},
		name:    name,
		model:   cfg.Model,
		baseURL: baseURL,
		apiKey:  cfg.APIKey,
	}, nil
}

func (p *Provider) Name() string         { return p.name }
func (p *Provider) SupportsTools() bool  { return true }
func (p *Provider) SupportsVision() bool { return true }
func (p *Provider) Capabilities() llm.ProviderCapabilities {
	return llm.DefaultCapabilities()
}

func (p *Provider) Chat(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	payload := p.buildRequest(req)
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal anthropic request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create anthropic request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic chat: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read anthropic response: %w", err)
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("anthropic chat failed: %s", strings.TrimSpace(string(respBody)))
	}

	var result anthropicResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("decode anthropic response: %w", err)
	}

	return &llm.ChatResponse{
		Message: convertAnthropicResponse(result),
		Usage: types.Usage{
			PromptTokens:     result.Usage.InputTokens,
			CompletionTokens: result.Usage.OutputTokens,
			TotalTokens:      result.Usage.InputTokens + result.Usage.OutputTokens,
		},
	}, nil
}

func (p *Provider) ChatStream(ctx context.Context, req *llm.ChatRequest) (llm.Stream, error) {
	payload := p.buildRequest(req)
	payload.Stream = true

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal anthropic stream request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create anthropic stream request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic stream: %w", err)
	}
	if resp.StatusCode >= 300 {
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("anthropic stream failed: %s", strings.TrimSpace(string(respBody)))
	}

	return &anthropicStream{
		resp:   resp,
		reader: bufio.NewReader(resp.Body),
		blocks: make(map[int]*types.ToolCall),
	}, nil
}

func (p *Provider) buildRequest(req *llm.ChatRequest) anthropicRequest {
	model := req.Model
	if model == "" {
		model = p.model
	}

	msgs, system := convertMessages(req.Messages, req.System)
	r := anthropicRequest{
		Model:     model,
		Messages:  msgs,
		System:    system,
		MaxTokens: req.MaxTokens,
		Tools:     convertTools(llm.AdaptToolDefs(p.Capabilities(), req.Tools)),
	}
	if r.MaxTokens <= 0 {
		r.MaxTokens = 4096
	}
	if req.Temperature > 0 {
		r.Temperature = &req.Temperature
	}
	if req.TopP > 0 {
		r.TopP = &req.TopP
	}
	if len(req.Stop) > 0 {
		r.StopSequences = req.Stop
	}
	return r
}

type anthropicRequest struct {
	Model         string             `json:"model"`
	System        string             `json:"system,omitempty"`
	Messages      []anthropicMessage `json:"messages"`
	MaxTokens     int                `json:"max_tokens"`
	Stream        bool               `json:"stream,omitempty"`
	Temperature   *float64           `json:"temperature,omitempty"`
	TopP          *float64           `json:"top_p,omitempty"`
	StopSequences []string           `json:"stop_sequences,omitempty"`
	Tools         []anthropicTool    `json:"tools,omitempty"`
}

type anthropicMessage struct {
	Role    string                  `json:"role"`
	Content []anthropicContentBlock `json:"content"`
}

type anthropicContentBlock struct {
	Type  string `json:"type"`
	Text  string `json:"text,omitempty"`
	ID    string `json:"id,omitempty"`
	Name  string `json:"name,omitempty"`
	Input any    `json:"input,omitempty"`

	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`

	Source *anthropicImageSource `json:"source,omitempty"`
}

type anthropicImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type anthropicResponse struct {
	Content []anthropicContentBlock `json:"content"`
	Usage   struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

type anthropicSSEPayload struct {
	Type  string `json:"type"`
	Index int    `json:"index"`
	Delta struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		PartialJSON string `json:"partial_json"`
	} `json:"delta"`
	ContentBlock struct {
		Type  string `json:"type"`
		ID    string `json:"id"`
		Name  string `json:"name"`
		Text  string `json:"text"`
		Input any    `json:"input"`
	} `json:"content_block"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

type anthropicStream struct {
	resp   *http.Response
	reader *bufio.Reader
	blocks map[int]*types.ToolCall
	usage  *types.Usage
	seen   bool
}

func (s *anthropicStream) Next() (types.StreamEvent, error) {
	for {
		_, data, err := readSSEEvent(s.reader)
		if err != nil {
			if err == io.EOF {
				if !s.seen {
					return types.StreamEvent{Type: types.EventError, Error: "anthropic stream ended without any events"}, io.EOF
				}
				return types.StreamEvent{Type: types.EventDone, Usage: s.usage}, io.EOF
			}
			return types.StreamEvent{}, err
		}
		if len(data) == 0 {
			continue
		}

		var payload anthropicSSEPayload
		if err := json.Unmarshal(data, &payload); err != nil {
			return types.StreamEvent{}, fmt.Errorf("decode anthropic stream event: %w", err)
		}
		s.seen = true

		switch payload.Type {
		case "message_start", "message_delta":
			if payload.Usage.InputTokens > 0 || payload.Usage.OutputTokens > 0 {
				s.usage = &types.Usage{
					PromptTokens:     payload.Usage.InputTokens,
					CompletionTokens: payload.Usage.OutputTokens,
					TotalTokens:      payload.Usage.InputTokens + payload.Usage.OutputTokens,
				}
			}
		case "content_block_start":
			if payload.ContentBlock.Type == "text" && payload.ContentBlock.Text != "" {
				return types.StreamEvent{Type: types.EventTextDelta, Content: payload.ContentBlock.Text}, nil
			}
			if payload.ContentBlock.Type == "tool_use" {
				params := "{}"
				if payload.ContentBlock.Input != nil {
					if b, err := json.Marshal(payload.ContentBlock.Input); err == nil {
						params = string(b)
					}
				}
				tc := &types.ToolCall{ID: payload.ContentBlock.ID, Name: payload.ContentBlock.Name, Params: params}
				s.blocks[payload.Index] = tc
				return types.StreamEvent{Type: types.EventToolCallStart, ToolCall: tc}, nil
			}
		case "content_block_delta":
			if payload.Delta.Type == "text_delta" && payload.Delta.Text != "" {
				return types.StreamEvent{Type: types.EventTextDelta, Content: payload.Delta.Text}, nil
			}
			if payload.Delta.Type == "input_json_delta" {
				if tc := s.blocks[payload.Index]; tc != nil {
					if tc.Params == "{}" {
						tc.Params = ""
					}
					tc.Params += payload.Delta.PartialJSON
					return types.StreamEvent{Type: types.EventToolCallDelta, Content: payload.Delta.PartialJSON, ToolCall: tc}, nil
				}
			}
		case "content_block_stop":
			if tc := s.blocks[payload.Index]; tc != nil {
				if tc.Params == "" {
					tc.Params = "{}"
				}
				delete(s.blocks, payload.Index)
				return types.StreamEvent{Type: types.EventToolCallEnd, ToolCall: tc}, nil
			}
		case "message_stop":
			return types.StreamEvent{Type: types.EventDone, Usage: s.usage}, nil
		case "error":
			msg := payload.Error.Message
			if msg == "" {
				msg = string(data)
			}
			return types.StreamEvent{Type: types.EventError, Error: msg}, nil
		}
	}
}

func (s *anthropicStream) Close() error {
	if s.resp != nil && s.resp.Body != nil {
		return s.resp.Body.Close()
	}
	return nil
}

func convertMessages(msgs []types.Message, system string) ([]anthropicMessage, string) {
	var result []anthropicMessage
	systemText := system

	for _, msg := range msgs {
		switch msg.Role {
		case types.RoleSystem:
			if text := msg.GetText(); text != "" {
				if systemText != "" {
					systemText += "\n\n"
				}
				systemText += text
			}
		case types.RoleUser, types.RoleAssistant:
			role := string(msg.Role)
			content := convertContent(msg.Content)
			if len(content) > 0 {
				result = append(result, anthropicMessage{Role: role, Content: content})
			}
		case types.RoleTool:
			content := convertToolResults(msg.Content)
			if len(content) > 0 {
				result = append(result, anthropicMessage{Role: "user", Content: content})
			}
		}
	}

	return result, systemText
}

func convertContent(blocks []types.ContentBlock) []anthropicContentBlock {
	var result []anthropicContentBlock
	for _, block := range blocks {
		switch block.Type {
		case types.ContentTypeText:
			if block.Text != "" {
				result = append(result, anthropicContentBlock{Type: "text", Text: block.Text})
			}
		case types.ContentTypeImage:
			if block.Image != nil {
				if block.Image.Base64 != "" {
					result = append(result, anthropicContentBlock{Type: "image", Source: &anthropicImageSource{Type: "base64", MediaType: block.Image.MediaType, Data: block.Image.Base64}})
				} else if block.Image.URL != "" {
					result = append(result, anthropicContentBlock{Type: "text", Text: "Image URL: " + block.Image.URL})
				}
			}
		case types.ContentTypeToolCall:
			if block.ToolCall != nil {
				var input any
				if block.ToolCall.Params != "" {
					_ = json.Unmarshal([]byte(block.ToolCall.Params), &input)
				}
				result = append(result, anthropicContentBlock{Type: "tool_use", ID: block.ToolCall.ID, Name: block.ToolCall.Name, Input: input})
			}
		}
	}
	return result
}

func convertToolResults(blocks []types.ContentBlock) []anthropicContentBlock {
	var result []anthropicContentBlock
	for _, block := range blocks {
		if block.Type == types.ContentTypeToolResult && block.ToolResult != nil {
			content := block.ToolResult.Content
			if block.ToolResult.Error != "" {
				content = block.ToolResult.Error
			}
			result = append(result, anthropicContentBlock{Type: "tool_result", ToolUseID: block.ToolResult.CallID, Content: content, IsError: block.ToolResult.Error != ""})
		}
	}
	return result
}

func convertTools(tools []types.ToolDef) []anthropicTool {
	result := make([]anthropicTool, 0, len(tools))
	for _, t := range tools {
		params := t.Parameters
		if len(params) == 0 {
			params = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		result = append(result, anthropicTool{Name: t.Name, Description: t.Description, InputSchema: params})
	}
	return result
}

func convertAnthropicResponse(resp anthropicResponse) types.Message {
	var content []types.ContentBlock
	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			if block.Text != "" {
				content = append(content, types.ContentBlock{Type: types.ContentTypeText, Text: block.Text})
			}
		case "tool_use":
			params := "{}"
			if block.Input != nil {
				if b, err := json.Marshal(block.Input); err == nil {
					params = string(b)
				}
			}
			content = append(content, types.ContentBlock{Type: types.ContentTypeToolCall, ToolCall: &types.ToolCall{ID: block.ID, Name: block.Name, Params: params}})
		}
	}
	return types.Message{Role: types.RoleAssistant, Content: content}
}

func readSSEEvent(reader *bufio.Reader) (string, []byte, error) {
	var eventName string
	var dataLines []string

	for {
		line, err := reader.ReadString('\n')
		if err != nil && !(err == io.EOF && len(line) > 0) {
			return "", nil, err
		}

		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			if len(dataLines) > 0 || eventName != "" {
				return eventName, []byte(strings.Join(dataLines, "\n")), nil
			}
			if err == io.EOF {
				return "", nil, io.EOF
			}
			continue
		}

		if strings.HasPrefix(line, "event:") {
			eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}

		if err == io.EOF {
			if len(dataLines) > 0 || eventName != "" {
				return eventName, []byte(strings.Join(dataLines, "\n")), nil
			}
			return "", nil, io.EOF
		}
	}
}

func normalizeBaseURL(raw string) string {
	baseURL := strings.TrimRight(raw, "/")
	if baseURL == "" {
		return defaultBaseURL
	}
	if !strings.HasSuffix(baseURL, "/v1") {
		baseURL += "/v1"
	}
	return baseURL
}

func init() {
	llm.DefaultRegistry.Register("anthropic", NewProvider)
}
