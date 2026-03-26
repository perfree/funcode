package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/perfree/funcode/internal/llm"
	"github.com/perfree/funcode/internal/logger"
	"github.com/perfree/funcode/pkg/types"
)

// Memory manages the conversation context for an agent.
type Memory struct {
	messages         []types.Message
	maxMessages      int
	summary          string
	compactThreshold int
	keepRecent       int
	onAdd            func(types.Message)
	onSummaryChange  func(string)
}

// NewMemory creates a new memory.
func NewMemory(maxMessages int) *Memory {
	if maxMessages <= 0 {
		maxMessages = 200
	}
	return &Memory{
		maxMessages:      maxMessages,
		compactThreshold: 50000,
		keepRecent:       12,
	}
}

// Add adds a message to memory.
func (m *Memory) Add(msg types.Message) {
	m.messages = append(m.messages, msg)
	if m.onAdd != nil {
		m.onAdd(msg)
	}
	if len(m.messages) > m.maxMessages {
		trimmed := make([]types.Message, 0, m.maxMessages)
		trimmed = append(trimmed, m.messages[0])
		start := len(m.messages) - m.maxMessages + 1
		trimmed = append(trimmed, m.messages[start:]...)
		m.messages = trimmed
	}
}

// Messages returns the summary plus recent messages.
func (m *Memory) Messages() []types.Message {
	if strings.TrimSpace(m.summary) == "" {
		return m.messages
	}
	msgs := make([]types.Message, 0, len(m.messages)+1)
	msgs = append(msgs, types.NewTextMessage(types.RoleSystem, "Conversation summary:\n"+m.summary))
	msgs = append(msgs, m.messages...)
	return msgs
}

// Clear resets memory.
func (m *Memory) Clear() {
	m.messages = nil
	m.summary = ""
}

// BuildMessages creates the message list with a new user input.
func (m *Memory) BuildMessages(input string) []types.Message {
	userMsg := types.NewTextMessage(types.RoleUser, input)
	m.Add(userMsg)
	return m.Messages()
}

// AddToolResults adds tool results as a message.
func (m *Memory) AddToolResults(results []types.ToolResult) {
	var content []types.ContentBlock
	for _, r := range results {
		content = append(content, types.ContentBlock{
			Type:       types.ContentTypeToolResult,
			ToolResult: &r,
		})
	}
	m.Add(types.Message{
		Role:    types.RoleTool,
		Content: content,
	})
}

func (m *Memory) SetHooks(onAdd func(types.Message), onSummaryChange func(string)) {
	m.onAdd = onAdd
	m.onSummaryChange = onSummaryChange
}

func (m *Memory) SetCompactThreshold(threshold int) {
	if threshold > 0 {
		m.compactThreshold = threshold
	}
}

func (m *Memory) Load(messages []types.Message, summary string) {
	m.messages = append([]types.Message(nil), messages...)
	m.summary = summary
}

func (m *Memory) Summary() string {
	return m.summary
}

func (m *Memory) DelegationContext() string {
	var b strings.Builder
	if strings.TrimSpace(m.summary) != "" {
		b.WriteString("Known project/context summary:\n")
		b.WriteString(strings.TrimSpace(m.summary))
		b.WriteString("\n\n")
	}

	start := 0
	if len(m.messages) > m.keepRecent {
		start = len(m.messages) - m.keepRecent
	}

	// Collect tool call summaries (files read, commands run)
	var toolSummaries []string
	for _, msg := range m.messages[start:] {
		for _, block := range msg.Content {
			if block.Type == types.ContentTypeToolCall && block.ToolCall != nil {
				toolSummaries = append(toolSummaries, summarizeToolCall(*block.ToolCall))
			}
		}
	}
	if len(toolSummaries) > 0 {
		b.WriteString("Tools already used (do NOT repeat these unless necessary):\n")
		for _, s := range toolSummaries {
			b.WriteString("- ")
			b.WriteString(s)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	if start < len(m.messages) {
		b.WriteString("Recent relevant conversation:\n")
		for _, msg := range m.messages[start:] {
			text := strings.TrimSpace(msg.GetText())
			if text == "" {
				continue
			}
			b.WriteString("- ")
			b.WriteString(string(msg.Role))
			b.WriteString(": ")
			b.WriteString(text)
			b.WriteString("\n")
		}
	}

	return strings.TrimSpace(b.String())
}

// summarizeToolCall creates a brief one-line summary of a tool call
func summarizeToolCall(tc types.ToolCall) string {
	switch tc.Name {
	case "Read":
		var p struct {
			FilePath string `json:"file_path"`
		}
		if json.Unmarshal([]byte(tc.Params), &p) == nil && p.FilePath != "" {
			return "Read " + p.FilePath
		}
	case "Glob":
		var p struct {
			Pattern string `json:"pattern"`
			Path    string `json:"path"`
		}
		if json.Unmarshal([]byte(tc.Params), &p) == nil {
			s := "Glob " + p.Pattern
			if p.Path != "" {
				s += " in " + p.Path
			}
			return s
		}
	case "Grep":
		var p struct {
			Pattern string `json:"pattern"`
			Path    string `json:"path"`
		}
		if json.Unmarshal([]byte(tc.Params), &p) == nil {
			s := "Grep " + p.Pattern
			if p.Path != "" {
				s += " in " + p.Path
			}
			return s
		}
	case "Bash":
		var p struct {
			Command string `json:"command"`
		}
		if json.Unmarshal([]byte(tc.Params), &p) == nil {
			cmd := p.Command
			if len(cmd) > 60 {
				cmd = cmd[:60] + "..."
			}
			return "Bash: " + cmd
		}
	case "Write":
		var p struct {
			FilePath string `json:"file_path"`
		}
		if json.Unmarshal([]byte(tc.Params), &p) == nil && p.FilePath != "" {
			return "Write " + p.FilePath
		}
	case "Edit":
		var p struct {
			FilePath string `json:"file_path"`
		}
		if json.Unmarshal([]byte(tc.Params), &p) == nil && p.FilePath != "" {
			return "Edit " + p.FilePath
		}
	}
	return tc.Name
}

func (m *Memory) Compact(ctx context.Context, provider llm.Provider, systemPrompt string) error {
	if provider == nil || !m.shouldCompact() || len(m.messages) <= m.keepRecent {
		return nil
	}

	logger.Info("memory compact triggered", "messages", len(m.messages), "estimated_tokens", estimateMessagesTokens(m.Messages()))

	cutoff := len(m.messages) - m.keepRecent
	older := append([]types.Message(nil), m.messages[:cutoff]...)
	recent := append([]types.Message(nil), m.messages[cutoff:]...)

	summary, err := m.compressMessages(ctx, provider, systemPrompt, older)
	if err != nil {
		logger.Error("memory compact failed", "error", err)
		return err
	}
	if strings.TrimSpace(summary) == "" {
		logger.Error("memory compact produced empty summary")
		return fmt.Errorf("context compression produced an empty summary")
	}

	if strings.TrimSpace(m.summary) != "" {
		summary = strings.TrimSpace(m.summary) + "\n\n" + summary
	}

	m.summary = summary
	m.messages = recent
	if m.onSummaryChange != nil {
		m.onSummaryChange(summary)
	}

	logger.Info("memory compact done", "summary_len", len(summary), "messages_kept", len(recent))
	return nil
}

func (m *Memory) shouldCompact() bool {
	return estimateMessagesTokens(m.Messages()) >= m.compactThreshold
}

func (m *Memory) compressMessages(ctx context.Context, provider llm.Provider, systemPrompt string, older []types.Message) (string, error) {
	prompt := `Summarize the prior conversation into durable working memory for an ongoing coding task.
Keep only information that matters for future execution.

Output sections:
1. Current goal
2. Confirmed constraints
3. Relevant files and symbols
4. Decisions made
5. Work completed
6. Remaining tasks
7. Risks / unknowns

Rules:
- Be concrete and concise
- Preserve file paths, function names, config keys, and explicit user requirements
- Omit chatter and repetitive reasoning`

	req := &llm.ChatRequest{
		System:   systemPrompt,
		Messages: append(older, types.NewTextMessage(types.RoleUser, prompt)),
	}
	resp, err := provider.Chat(ctx, req)
	if err != nil {
		return "", fmt.Errorf("context compression failed: %w", err)
	}
	return strings.TrimSpace(resp.Message.GetText()), nil
}

func estimateMessagesTokens(messages []types.Message) int {
	total := 0
	for _, msg := range messages {
		for _, block := range msg.Content {
			switch block.Type {
			case types.ContentTypeText:
				total += estimateStringTokens(block.Text)
			case types.ContentTypeToolCall:
				if block.ToolCall != nil {
					total += estimateStringTokens(block.ToolCall.Name)
					total += estimateStringTokens(block.ToolCall.Params)
				}
			case types.ContentTypeToolResult:
				if block.ToolResult != nil {
					total += estimateStringTokens(block.ToolResult.Content)
					total += estimateStringTokens(block.ToolResult.Error)
				}
			}
		}
	}
	return total
}

func estimateStringTokens(s string) int {
	if s == "" {
		return 0
	}
	return (len(s) + 3) / 4
}
