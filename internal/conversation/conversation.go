package conversation

import (
	"time"

	"github.com/perfree/funcode/pkg/types"
)

// Session represents a conversation session
type Session struct {
	ID           string    `json:"id"`
	ProjectHash  string    `json:"project_hash"`
	ProjectPath  string    `json:"project_path"`
	Title        string    `json:"title,omitempty"`
	Summary      string    `json:"summary,omitempty"`
	Model        string    `json:"model,omitempty"`
	Role         string    `json:"role,omitempty"`
	GitBranch    string    `json:"git_branch,omitempty"`
	MessageCount int       `json:"message_count"`
	TokenCount   int       `json:"token_count"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Conversation holds the active session and messages
type Conversation struct {
	Session  *Session
	Messages []*StoredMessage
}

// AddMessage adds a message to the conversation
func (c *Conversation) AddMessage(msg types.Message) *StoredMessage {
	id := generateID()
	sm := FromMessage(id, msg)
	c.Messages = append(c.Messages, sm)
	c.Session.MessageCount++
	c.Session.UpdatedAt = time.Now()
	return sm
}

// GetMessages returns all messages as types.Message
func (c *Conversation) GetMessages() []types.Message {
	msgs := make([]types.Message, len(c.Messages))
	for i, sm := range c.Messages {
		msgs[i] = sm.ToMessage()
	}
	return msgs
}

// LastN returns the last N messages
func (c *Conversation) LastN(n int) []types.Message {
	msgs := c.GetMessages()
	if len(msgs) <= n {
		return msgs
	}
	return msgs[len(msgs)-n:]
}
