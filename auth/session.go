// Package auth provides authentication and session management for appbase applications.
package auth

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	appdb "github.com/michaelwinser/appbase/db"
)

// Session represents an authenticated user session.
type Session struct {
	ID        string
	UserID    string
	Email     string
	ExpiresAt time.Time
	CreatedAt time.Time
}

// IsExpired returns true if the session has expired.
func (s *Session) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
}

// SessionStore manages session persistence.
type SessionStore struct {
	db *appdb.DB
}

// NewSessionStore creates a session store and ensures the sessions table exists.
func NewSessionStore(db *appdb.DB) (*SessionStore, error) {
	err := db.Migrate(`
		CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			email TEXT NOT NULL,
			expires_at TEXT NOT NULL,
			created_at TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
		CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);
	`)
	if err != nil {
		return nil, fmt.Errorf("creating sessions table: %w", err)
	}
	return &SessionStore{db: db}, nil
}

// Create inserts a new session and returns its ID.
func (s *SessionStore) Create(userID, email string, ttl time.Duration) (*Session, error) {
	session := &Session{
		ID:        generateSessionID(),
		UserID:    userID,
		Email:     email,
		ExpiresAt: time.Now().Add(ttl),
		CreatedAt: time.Now(),
	}
	_, err := s.db.Exec(
		`INSERT INTO sessions (id, user_id, email, expires_at, created_at) VALUES (?, ?, ?, ?, ?)`,
		session.ID, session.UserID, session.Email,
		session.ExpiresAt.Format(time.RFC3339),
		session.CreatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("creating session: %w", err)
	}
	return session, nil
}

// Get retrieves a session by ID. Returns nil if not found.
func (s *SessionStore) Get(id string) (*Session, error) {
	row := s.db.QueryRow(
		`SELECT id, user_id, email, expires_at, created_at FROM sessions WHERE id = ?`, id,
	)
	var session Session
	var expiresAt, createdAt string
	err := row.Scan(&session.ID, &session.UserID, &session.Email, &expiresAt, &createdAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting session: %w", err)
	}
	session.ExpiresAt, _ = time.Parse(time.RFC3339, expiresAt)
	session.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &session, nil
}

// Delete removes a session by ID.
func (s *SessionStore) Delete(id string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	return err
}

// DeleteExpired removes all expired sessions.
func (s *SessionStore) DeleteExpired() error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE expires_at < ?`, time.Now().Format(time.RFC3339))
	return err
}

// DeleteByUser removes all sessions for a user.
func (s *SessionStore) DeleteByUser(userID string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE user_id = ?`, userID)
	return err
}

func generateSessionID() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("failed to generate session ID: " + err.Error())
	}
	return hex.EncodeToString(b)
}
