// Package auth provides authentication and session management for appbase applications.
package auth

import (
	"crypto/rand"
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
	// OAuth tokens (empty for local/desktop sessions without OAuth).
	AccessToken  string
	RefreshToken string
	TokenExpiry  time.Time
}

// IsExpired returns true if the session has expired.
func (s *Session) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
}

// TokenExpired returns true if the OAuth access token has expired.
// Returns true if no token expiry is set (no tokens stored).
func (s *Session) TokenExpired() bool {
	return s.TokenExpiry.IsZero() || time.Now().After(s.TokenExpiry)
}

// sessionBackend abstracts session persistence across SQL and Firestore.
type sessionBackend interface {
	Init() error
	Create(session *Session) error
	Get(id string) (*Session, error)
	UpdateTokens(sessionID, accessToken, refreshToken string, tokenExpiry time.Time) error
	Delete(id string) error
	DeleteExpired() error
	DeleteByUser(userID string) error
}

// SessionStore manages session persistence.
type SessionStore struct {
	backend sessionBackend
}

// NewSessionStore creates a session store backed by the given database.
// Automatically selects the right backend (SQL or Firestore) based on db.StoreType().
func NewSessionStore(db *appdb.DB) (*SessionStore, error) {
	var backend sessionBackend
	if db.IsSQL() {
		backend = &sqlSessionBackend{db: db}
	} else {
		backend = &firestoreSessionBackend{db: db}
	}

	if err := backend.Init(); err != nil {
		return nil, fmt.Errorf("initializing session store: %w", err)
	}
	return &SessionStore{backend: backend}, nil
}

// Create inserts a new session.
func (s *SessionStore) Create(userID, email string, ttl time.Duration) (*Session, error) {
	session := &Session{
		ID:        generateSessionID(),
		UserID:    userID,
		Email:     email,
		ExpiresAt: time.Now().Add(ttl),
		CreatedAt: time.Now(),
	}
	if err := s.backend.Create(session); err != nil {
		return nil, err
	}
	return session, nil
}

// Get retrieves a session by ID. Returns nil if not found.
func (s *SessionStore) Get(id string) (*Session, error) {
	return s.backend.Get(id)
}

// UpdateTokens stores OAuth tokens on an existing session.
func (s *SessionStore) UpdateTokens(sessionID, accessToken, refreshToken string, tokenExpiry time.Time) error {
	return s.backend.UpdateTokens(sessionID, accessToken, refreshToken, tokenExpiry)
}

// Delete removes a session by ID.
func (s *SessionStore) Delete(id string) error {
	return s.backend.Delete(id)
}

// DeleteExpired removes all expired sessions.
func (s *SessionStore) DeleteExpired() error {
	return s.backend.DeleteExpired()
}

// DeleteByUser removes all sessions for a user.
func (s *SessionStore) DeleteByUser(userID string) error {
	return s.backend.DeleteByUser(userID)
}

func generateSessionID() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("failed to generate session ID: " + err.Error())
	}
	return hex.EncodeToString(b)
}
