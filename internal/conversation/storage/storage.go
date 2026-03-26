package storage

import "github.com/perfree/funcode/internal/conversation"

// Storage defines the interface for conversation persistence
type Storage interface {
	// SaveMessage appends a message to the session's storage
	SaveMessage(sessionID string, msg *conversation.StoredMessage) error

	// LoadMessages loads all messages for a session
	LoadMessages(sessionID string) ([]*conversation.StoredMessage, error)

	// CreateSession creates a new session record
	CreateSession(session *conversation.Session) error

	// UpdateSession updates session metadata
	UpdateSession(session *conversation.Session) error

	// GetLatestSession returns the latest session for a project
	GetLatestSession(projectHash string) (*conversation.Session, error)

	// ListSessions lists sessions for a project
	ListSessions(projectHash string, limit int) ([]*conversation.Session, error)

	// Close closes the storage
	Close() error
}
