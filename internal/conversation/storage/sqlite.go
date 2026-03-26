package storage

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/perfree/funcode/internal/conversation"
	_ "modernc.org/sqlite"
)

// SQLiteIndex provides session indexing and search via SQLite
type SQLiteIndex struct {
	db *sql.DB
}

// NewSQLiteIndex creates a new SQLite index
func NewSQLiteIndex(dbPath string) (*SQLiteIndex, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite: %w", err)
	}

	// Enable WAL mode for better concurrent access
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, err
	}

	idx := &SQLiteIndex{db: db}
	if err := idx.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrating sqlite: %w", err)
	}

	return idx, nil
}

func (idx *SQLiteIndex) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS sessions (
		id           TEXT PRIMARY KEY,
		project_hash TEXT NOT NULL,
		project_path TEXT NOT NULL,
		title        TEXT DEFAULT '',
		summary      TEXT DEFAULT '',
		model        TEXT DEFAULT '',
		role         TEXT DEFAULT '',
		git_branch   TEXT DEFAULT '',
		message_count INTEGER DEFAULT 0,
		token_count  INTEGER DEFAULT 0,
		created_at   DATETIME NOT NULL,
		updated_at   DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_sessions_project ON sessions(project_hash);
	CREATE INDEX IF NOT EXISTS idx_sessions_updated ON sessions(updated_at DESC);
	`
	_, err := idx.db.Exec(schema)
	return err
}

// IndexSession stores/updates a session in the index
func (idx *SQLiteIndex) IndexSession(session *conversation.Session) error {
	_, err := idx.db.Exec(`
		INSERT INTO sessions (id, project_hash, project_path, title, summary, model, role, git_branch, message_count, token_count, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			title = excluded.title,
			summary = excluded.summary,
			message_count = excluded.message_count,
			token_count = excluded.token_count,
			updated_at = excluded.updated_at
	`,
		session.ID, session.ProjectHash, session.ProjectPath,
		session.Title, session.Summary, session.Model, session.Role, session.GitBranch,
		session.MessageCount, session.TokenCount,
		session.CreatedAt.Format(time.RFC3339), session.UpdatedAt.Format(time.RFC3339),
	)
	return err
}

// GetLatestSession returns the most recent session for a project
func (idx *SQLiteIndex) GetLatestSession(projectHash string) (*conversation.Session, error) {
	row := idx.db.QueryRow(`
		SELECT id, project_hash, project_path, title, summary, model, role, git_branch, message_count, token_count, created_at, updated_at
		FROM sessions WHERE project_hash = ? ORDER BY updated_at DESC LIMIT 1
	`, projectHash)

	return scanSession(row)
}

// ListSessions lists sessions for a project
func (idx *SQLiteIndex) ListSessions(projectHash string, limit int) ([]*conversation.Session, error) {
	query := "SELECT id, project_hash, project_path, title, summary, model, role, git_branch, message_count, token_count, created_at, updated_at FROM sessions"
	args := []any{}

	if projectHash != "" {
		query += " WHERE project_hash = ?"
		args = append(args, projectHash)
	}
	query += " ORDER BY updated_at DESC"
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := idx.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*conversation.Session
	for rows.Next() {
		s, err := scanSessionRows(rows)
		if err != nil {
			continue
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

// Search searches sessions by keyword
func (idx *SQLiteIndex) Search(query string) ([]*conversation.Session, error) {
	pattern := "%" + query + "%"
	rows, err := idx.db.Query(`
		SELECT id, project_hash, project_path, title, summary, model, role, git_branch, message_count, token_count, created_at, updated_at
		FROM sessions WHERE title LIKE ? OR summary LIKE ? ORDER BY updated_at DESC LIMIT 20
	`, pattern, pattern)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*conversation.Session
	for rows.Next() {
		s, err := scanSessionRows(rows)
		if err != nil {
			continue
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

// Close closes the database
func (idx *SQLiteIndex) Close() error {
	return idx.db.Close()
}

type scannable interface {
	Scan(dest ...any) error
}

func scanSession(row *sql.Row) (*conversation.Session, error) {
	s := &conversation.Session{}
	var createdAt, updatedAt string
	err := row.Scan(&s.ID, &s.ProjectHash, &s.ProjectPath, &s.Title, &s.Summary,
		&s.Model, &s.Role, &s.GitBranch, &s.MessageCount, &s.TokenCount,
		&createdAt, &updatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	s.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	s.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return s, nil
}

func scanSessionRows(rows *sql.Rows) (*conversation.Session, error) {
	s := &conversation.Session{}
	var createdAt, updatedAt string
	err := rows.Scan(&s.ID, &s.ProjectHash, &s.ProjectPath, &s.Title, &s.Summary,
		&s.Model, &s.Role, &s.GitBranch, &s.MessageCount, &s.TokenCount,
		&createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	s.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	s.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return s, nil
}
