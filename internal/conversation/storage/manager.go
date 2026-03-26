package storage

import (
	"path/filepath"
	"time"

	"github.com/perfree/funcode/internal/config"
	"github.com/perfree/funcode/internal/conversation"
	"github.com/perfree/funcode/pkg/types"
)

// Manager coordinates JSONL message storage and SQLite session indexing.
type Manager struct {
	jsonl  *JSONLStorage
	sqlite *SQLiteIndex
}

// NewManager creates a conversation manager backed by ~/.funcode storage.
func NewManager() (*Manager, error) {
	jsonl := NewJSONLStorage(filepath.Join(config.GlobalConfigDir(), "conversations"))
	sqlite, err := NewSQLiteIndex(filepath.Join(config.GlobalConfigDir(), "funcode.db"))
	if err != nil {
		return nil, err
	}
	return &Manager{
		jsonl:  jsonl,
		sqlite: sqlite,
	}, nil
}

// OpenOrCreate returns the latest project session or creates a new one.
func (m *Manager) OpenOrCreate(projectPath, model, role string, resume bool) (*conversation.Conversation, error) {
	projectHash := conversation.HashProjectPath(projectPath)
	if resume {
		session, err := m.sqlite.GetLatestSession(projectHash)
		if err != nil {
			return nil, err
		}
		if session != nil {
			messages, err := m.jsonl.LoadMessages(session.ID)
			if err != nil {
				return nil, err
			}
			return &conversation.Conversation{
				Session:  session,
				Messages: messages,
			}, nil
		}
	}

	now := time.Now()
	session := &conversation.Session{
		ID:          conversation.GenerateSessionID(),
		ProjectHash: projectHash,
		ProjectPath: projectPath,
		Title:       filepath.Base(projectPath),
		Model:       model,
		Role:        role,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := m.jsonl.CreateSession(session); err != nil {
		return nil, err
	}
	if err := m.sqlite.IndexSession(session); err != nil {
		return nil, err
	}
	return &conversation.Conversation{
		Session:  session,
		Messages: []*conversation.StoredMessage{},
	}, nil
}

// Append stores a single message and updates the session index.
func (m *Manager) Append(conv *conversation.Conversation, msg types.Message) error {
	if conv == nil || conv.Session == nil {
		return nil
	}

	stored := conv.AddMessage(msg)
	conv.Session.TokenCount += estimateMessageTokens(msg)

	if err := m.jsonl.SaveMessage(conv.Session.ID, stored); err != nil {
		return err
	}
	if err := m.jsonl.UpdateSession(conv.Session); err != nil {
		return err
	}
	return m.sqlite.IndexSession(conv.Session)
}

// UpdateSummary persists the latest session summary.
func (m *Manager) UpdateSummary(conv *conversation.Conversation, summary string) error {
	if conv == nil || conv.Session == nil {
		return nil
	}
	conv.Session.Summary = summary
	conv.Session.UpdatedAt = time.Now()
	if err := m.jsonl.UpdateSession(conv.Session); err != nil {
		return err
	}
	return m.sqlite.IndexSession(conv.Session)
}

// Close closes the SQLite index.
func (m *Manager) Close() error {
	if m.sqlite != nil {
		return m.sqlite.Close()
	}
	return nil
}

func estimateMessageTokens(msg types.Message) int {
	total := 0
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
	return total
}

func estimateStringTokens(s string) int {
	if s == "" {
		return 0
	}
	return (len(s) + 3) / 4
}
