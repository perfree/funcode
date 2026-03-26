package storage

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/perfree/funcode/internal/conversation"
)

// JSONLStorage stores messages as JSONL files
type JSONLStorage struct {
	baseDir string // ~/.funcode/conversations/
}

// NewJSONLStorage creates a new JSONL storage
func NewJSONLStorage(baseDir string) *JSONLStorage {
	return &JSONLStorage{baseDir: baseDir}
}

func (s *JSONLStorage) sessionDir(sessionID string) string {
	return filepath.Join(s.baseDir, sessionID)
}

func (s *JSONLStorage) messagesFile(sessionID string) string {
	return filepath.Join(s.sessionDir(sessionID), "messages.jsonl")
}

func (s *JSONLStorage) metaFile(sessionID string) string {
	return filepath.Join(s.sessionDir(sessionID), "meta.json")
}

// SaveMessage appends a message to the JSONL file
func (s *JSONLStorage) SaveMessage(sessionID string, msg *conversation.StoredMessage) error {
	dir := s.sessionDir(sessionID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating session dir: %w", err)
	}

	f, err := os.OpenFile(s.messagesFile(sessionID), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("opening messages file: %w", err)
	}
	defer f.Close()

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshaling message: %w", err)
	}

	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("writing message: %w", err)
	}

	return nil
}

// LoadMessages reads all messages from the JSONL file
func (s *JSONLStorage) LoadMessages(sessionID string) ([]*conversation.StoredMessage, error) {
	f, err := os.Open(s.messagesFile(sessionID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("opening messages file: %w", err)
	}
	defer f.Close()

	var messages []*conversation.StoredMessage
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 10*1024*1024), 10*1024*1024) // 10MB buffer

	for scanner.Scan() {
		var msg conversation.StoredMessage
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			continue // skip malformed lines
		}
		messages = append(messages, &msg)
	}

	return messages, scanner.Err()
}

// CreateSession writes session metadata
func (s *JSONLStorage) CreateSession(session *conversation.Session) error {
	dir := s.sessionDir(session.ID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return s.writeSessionMeta(session)
}

// UpdateSession updates session metadata
func (s *JSONLStorage) UpdateSession(session *conversation.Session) error {
	return s.writeSessionMeta(session)
}

func (s *JSONLStorage) writeSessionMeta(session *conversation.Session) error {
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.metaFile(session.ID), data, 0644)
}

// GetLatestSession returns the most recent session for a project
func (s *JSONLStorage) GetLatestSession(projectHash string) (*conversation.Session, error) {
	sessions, err := s.ListSessions(projectHash, 1)
	if err != nil {
		return nil, err
	}
	if len(sessions) == 0 {
		return nil, nil
	}
	return sessions[0], nil
}

// ListSessions lists sessions by scanning directories
func (s *JSONLStorage) ListSessions(projectHash string, limit int) ([]*conversation.Session, error) {
	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var sessions []*conversation.Session
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		metaPath := filepath.Join(s.baseDir, entry.Name(), "meta.json")
		data, err := os.ReadFile(metaPath)
		if err != nil {
			continue
		}

		var session conversation.Session
		if err := json.Unmarshal(data, &session); err != nil {
			continue
		}

		if projectHash == "" || session.ProjectHash == projectHash {
			sessions = append(sessions, &session)
		}

		if limit > 0 && len(sessions) >= limit {
			break
		}
	}

	return sessions, nil
}

// Close is a no-op for JSONL storage
func (s *JSONLStorage) Close() error {
	return nil
}
