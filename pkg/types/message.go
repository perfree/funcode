package types

// Role represents a message role
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// ContentType represents the type of content block
type ContentType string

const (
	ContentTypeText       ContentType = "text"
	ContentTypeImage      ContentType = "image"
	ContentTypeToolCall   ContentType = "tool_call"
	ContentTypeToolResult ContentType = "tool_result"
)

// Message is the unified message format
type Message struct {
	Role    Role           `json:"role"`
	Content []ContentBlock `json:"content"`
}

// ContentBlock represents a block of content
type ContentBlock struct {
	Type       ContentType `json:"type"`
	Text       string      `json:"text,omitempty"`
	Image      *ImageData  `json:"image,omitempty"`
	ToolCall   *ToolCall   `json:"tool_call,omitempty"`
	ToolResult *ToolResult `json:"tool_result,omitempty"`
}

// ImageData represents image content
type ImageData struct {
	URL       string `json:"url,omitempty"`
	Base64    string `json:"base64,omitempty"`
	MediaType string `json:"media_type,omitempty"`
}

// ToolCall represents a tool call from the model
type ToolCall struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Params string `json:"params"` // JSON string
}

// ToolResult represents the result of a tool execution
type ToolResult struct {
	CallID  string `json:"call_id"`
	Content string `json:"content"`
	Error   string `json:"error,omitempty"`
}

// NewTextMessage creates a simple text message
func NewTextMessage(role Role, text string) Message {
	return Message{
		Role: role,
		Content: []ContentBlock{
			{Type: ContentTypeText, Text: text},
		},
	}
}

// GetText extracts all text content from a message
func (m *Message) GetText() string {
	var text string
	for _, block := range m.Content {
		if block.Type == ContentTypeText {
			text += block.Text
		}
	}
	return text
}

// GetToolCalls extracts all tool calls from a message
func (m *Message) GetToolCalls() []ToolCall {
	var calls []ToolCall
	for _, block := range m.Content {
		if block.Type == ContentTypeToolCall && block.ToolCall != nil {
			calls = append(calls, *block.ToolCall)
		}
	}
	return calls
}
