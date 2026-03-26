package google

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

const defaultBaseURL = "https://generativelanguage.googleapis.com/v1beta"

type Provider struct {
	client  *http.Client
	name    string
	model   string
	baseURL string
	apiKey  string
}

func NewProvider(cfg *llm.ProviderConfig) (llm.Provider, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("Google API key is required")
	}

	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	name := "google"
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
	model := req.Model
	if model == "" {
		model = p.model
	}
	if model == "" {
		model = "gemini-2.0-flash"
	}

	payload := p.buildRequest(req)
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal google request: %w", err)
	}

	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s", p.baseURL, model, p.apiKey)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create google request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("google chat: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read google response: %w", err)
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("google chat failed: %s", strings.TrimSpace(string(respBody)))
	}

	var result googleResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("decode google response: %w", err)
	}
	if len(result.Candidates) == 0 {
		return nil, fmt.Errorf("google: no candidates returned")
	}

	return &llm.ChatResponse{
		Message: convertGoogleCandidate(result.Candidates[0]),
		Usage: types.Usage{
			PromptTokens:     result.UsageMetadata.PromptTokenCount,
			CompletionTokens: result.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      result.UsageMetadata.TotalTokenCount,
		},
	}, nil
}

func (p *Provider) ChatStream(ctx context.Context, req *llm.ChatRequest) (llm.Stream, error) {
	model := req.Model
	if model == "" {
		model = p.model
	}
	if model == "" {
		model = "gemini-2.0-flash"
	}

	payload := p.buildRequest(req)
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal google stream request: %w", err)
	}

	url := fmt.Sprintf("%s/models/%s:streamGenerateContent?alt=sse&key=%s", p.baseURL, model, p.apiKey)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create google stream request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("google stream: %w", err)
	}
	if resp.StatusCode >= 300 {
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("google stream failed: %s", strings.TrimSpace(string(respBody)))
	}

	return &googleStream{
		resp:   resp,
		reader: bufio.NewReader(resp.Body),
	}, nil
}

type googleRequest struct {
	Contents          []googleContent         `json:"contents"`
	SystemInstruction *googleContent          `json:"systemInstruction,omitempty"`
	Tools             []googleToolWrapper     `json:"tools,omitempty"`
	GenerationConfig  *googleGenerationConfig `json:"generationConfig,omitempty"`
}

type googleContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []googlePart `json:"parts"`
}

type googlePart struct {
	Text             string                  `json:"text,omitempty"`
	InlineData       *googleInlineData       `json:"inlineData,omitempty"`
	FileData         *googleFileData         `json:"fileData,omitempty"`
	FunctionCall     *googleFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *googleFunctionResponse `json:"functionResponse,omitempty"`
}

type googleInlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

type googleFileData struct {
	MimeType string `json:"mimeType,omitempty"`
	FileURI  string `json:"fileUri"`
}

type googleFunctionCall struct {
	Name string `json:"name"`
	Args any    `json:"args,omitempty"`
}

type googleFunctionResponse struct {
	Name     string `json:"name"`
	Response any    `json:"response"`
}

type googleToolWrapper struct {
	FunctionDeclarations []googleFunctionDeclaration `json:"functionDeclarations"`
}

type googleFunctionDeclaration struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type googleGenerationConfig struct {
	Temperature     *float64 `json:"temperature,omitempty"`
	TopP            *float64 `json:"topP,omitempty"`
	MaxOutputTokens int      `json:"maxOutputTokens,omitempty"`
	StopSequences   []string `json:"stopSequences,omitempty"`
}

type googleCandidate struct {
	Content      googleContent `json:"content"`
	FinishReason string        `json:"finishReason"`
}

type googleResponse struct {
	Candidates    []googleCandidate `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		TotalTokenCount      int `json:"totalTokenCount"`
	} `json:"usageMetadata"`
}

type googleStream struct {
	resp    *http.Response
	reader  *bufio.Reader
	pending []types.StreamEvent
	usage   *types.Usage
}

type googleStreamChunk struct {
	Candidates    []googleCandidate `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		TotalTokenCount      int `json:"totalTokenCount"`
	} `json:"usageMetadata"`
}

func (s *googleStream) Next() (types.StreamEvent, error) {
	if len(s.pending) > 0 {
		evt := s.pending[0]
		s.pending = s.pending[1:]
		return evt, nil
	}

	for {
		_, data, err := readSSEEvent(s.reader)
		if err != nil {
			if err == io.EOF {
				return types.StreamEvent{Type: types.EventDone, Usage: s.usage}, io.EOF
			}
			return types.StreamEvent{}, err
		}
		if len(data) == 0 {
			continue
		}

		var chunk googleStreamChunk
		if err := json.Unmarshal(data, &chunk); err != nil {
			return types.StreamEvent{}, fmt.Errorf("decode google stream chunk: %w", err)
		}

		if chunk.UsageMetadata.TotalTokenCount > 0 || chunk.UsageMetadata.PromptTokenCount > 0 || chunk.UsageMetadata.CandidatesTokenCount > 0 {
			s.usage = &types.Usage{
				PromptTokens:     chunk.UsageMetadata.PromptTokenCount,
				CompletionTokens: chunk.UsageMetadata.CandidatesTokenCount,
				TotalTokens:      chunk.UsageMetadata.TotalTokenCount,
			}
		}

		for _, candidate := range chunk.Candidates {
			for _, part := range candidate.Content.Parts {
				if part.Text != "" {
					s.pending = append(s.pending, types.StreamEvent{Type: types.EventTextDelta, Content: part.Text})
				}
				if part.FunctionCall != nil {
					params := "{}"
					if part.FunctionCall.Args != nil {
						if b, err := json.Marshal(part.FunctionCall.Args); err == nil {
							params = string(b)
						}
					}
					tc := types.ToolCall{ID: part.FunctionCall.Name, Name: part.FunctionCall.Name, Params: params}
					s.pending = append(s.pending,
						types.StreamEvent{Type: types.EventToolCallStart, ToolCall: &tc},
						types.StreamEvent{Type: types.EventToolCallEnd, ToolCall: &tc},
					)
				}
			}
			if candidate.FinishReason != "" {
				s.pending = append(s.pending, types.StreamEvent{Type: types.EventDone, Usage: s.usage})
			}
		}

		if len(s.pending) > 0 {
			evt := s.pending[0]
			s.pending = s.pending[1:]
			return evt, nil
		}
	}
}

func (s *googleStream) Close() error {
	if s.resp != nil && s.resp.Body != nil {
		return s.resp.Body.Close()
	}
	return nil
}

func (p *Provider) buildRequest(req *llm.ChatRequest) googleRequest {
	contents, system := convertMessages(req.Messages, req.System)
	r := googleRequest{Contents: contents}
	if system != "" {
		r.SystemInstruction = &googleContent{Parts: []googlePart{{Text: system}}}
	}
	toolDefs := llm.AdaptToolDefs(p.Capabilities(), req.Tools)
	if len(toolDefs) > 0 {
		r.Tools = []googleToolWrapper{{FunctionDeclarations: convertTools(toolDefs)}}
	}

	cfg := &googleGenerationConfig{}
	if req.Temperature > 0 {
		cfg.Temperature = &req.Temperature
	}
	if req.TopP > 0 {
		cfg.TopP = &req.TopP
	}
	if req.MaxTokens > 0 {
		cfg.MaxOutputTokens = req.MaxTokens
	}
	if len(req.Stop) > 0 {
		cfg.StopSequences = req.Stop
	}
	if cfg.Temperature != nil || cfg.TopP != nil || cfg.MaxOutputTokens > 0 || len(cfg.StopSequences) > 0 {
		r.GenerationConfig = cfg
	}
	return r
}

func convertMessages(msgs []types.Message, system string) ([]googleContent, string) {
	var contents []googleContent
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
		case types.RoleUser:
			if parts := convertParts(msg.Content); len(parts) > 0 {
				contents = append(contents, googleContent{Role: "user", Parts: parts})
			}
		case types.RoleAssistant:
			if parts := convertParts(msg.Content); len(parts) > 0 {
				contents = append(contents, googleContent{Role: "model", Parts: parts})
			}
		case types.RoleTool:
			if parts := convertToolResponseParts(msg.Content); len(parts) > 0 {
				contents = append(contents, googleContent{Role: "user", Parts: parts})
			}
		}
	}

	return contents, systemText
}

func convertParts(blocks []types.ContentBlock) []googlePart {
	var parts []googlePart
	for _, block := range blocks {
		switch block.Type {
		case types.ContentTypeText:
			if block.Text != "" {
				parts = append(parts, googlePart{Text: block.Text})
			}
		case types.ContentTypeImage:
			if block.Image != nil {
				if block.Image.Base64 != "" {
					parts = append(parts, googlePart{InlineData: &googleInlineData{MimeType: block.Image.MediaType, Data: block.Image.Base64}})
				} else if block.Image.URL != "" {
					parts = append(parts, googlePart{FileData: &googleFileData{MimeType: block.Image.MediaType, FileURI: block.Image.URL}})
				}
			}
		case types.ContentTypeToolCall:
			if block.ToolCall != nil {
				var args any
				if block.ToolCall.Params != "" {
					_ = json.Unmarshal([]byte(block.ToolCall.Params), &args)
				}
				parts = append(parts, googlePart{FunctionCall: &googleFunctionCall{Name: block.ToolCall.Name, Args: args}})
			}
		}
	}
	return parts
}

func convertToolResponseParts(blocks []types.ContentBlock) []googlePart {
	var parts []googlePart
	for _, block := range blocks {
		if block.Type == types.ContentTypeToolResult && block.ToolResult != nil {
			payload := map[string]any{"content": block.ToolResult.Content}
			if block.ToolResult.Error != "" {
				payload["error"] = block.ToolResult.Error
			}
			parts = append(parts, googlePart{FunctionResponse: &googleFunctionResponse{Name: block.ToolResult.CallID, Response: payload}})
		}
	}
	return parts
}

func convertTools(tools []types.ToolDef) []googleFunctionDeclaration {
	result := make([]googleFunctionDeclaration, 0, len(tools))
	for _, t := range tools {
		params := t.Parameters
		if len(params) == 0 {
			params = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		result = append(result, googleFunctionDeclaration{Name: t.Name, Description: t.Description, Parameters: params})
	}
	return result
}

func convertGoogleCandidate(candidate googleCandidate) types.Message {
	var content []types.ContentBlock
	for _, part := range candidate.Content.Parts {
		if part.Text != "" {
			content = append(content, types.ContentBlock{Type: types.ContentTypeText, Text: part.Text})
		}
		if part.FunctionCall != nil {
			params := "{}"
			if part.FunctionCall.Args != nil {
				if b, err := json.Marshal(part.FunctionCall.Args); err == nil {
					params = string(b)
				}
			}
			content = append(content, types.ContentBlock{Type: types.ContentTypeToolCall, ToolCall: &types.ToolCall{ID: part.FunctionCall.Name, Name: part.FunctionCall.Name, Params: params}})
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

func init() {
	llm.DefaultRegistry.Register("google", NewProvider)
}
