package auth

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// CLILogin represents a pending CLI login request.
type CLILogin struct {
	Token     string    // Unique token the CLI uses to poll
	State     string    // OAuth state parameter (links callback to this login)
	SessionID string    // Populated after successful OAuth callback
	CreatedAt time.Time
}

// CLILoginStore manages pending CLI login requests.
// In-memory with expiry — no persistence needed.
type CLILoginStore struct {
	mu      sync.Mutex
	pending map[string]*CLILogin // keyed by state (for callback lookup)
	byToken map[string]*CLILogin // keyed by token (for polling)
}

// NewCLILoginStore creates a new store.
func NewCLILoginStore() *CLILoginStore {
	return &CLILoginStore{
		pending: make(map[string]*CLILogin),
		byToken: make(map[string]*CLILogin),
	}
}

// Create generates a new pending CLI login and returns the token and state.
func (s *CLILoginStore) Create() (token, state string) {
	token = generateToken()
	state = "cli:" + generateToken()

	login := &CLILogin{
		Token:     token,
		State:     state,
		CreatedAt: time.Now(),
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending[state] = login
	s.byToken[token] = login

	// Clean up expired entries
	s.cleanupLocked()

	return token, state
}

// Complete marks a CLI login as complete by setting the session ID.
// Called from the OAuth callback when the state starts with "cli:".
func (s *CLILoginStore) Complete(state, sessionID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	login, ok := s.pending[state]
	if !ok {
		return false
	}
	login.SessionID = sessionID
	return true
}

// Poll checks if a CLI login has completed. Returns the session ID
// or empty string if still pending.
func (s *CLILoginStore) Poll(token string) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	login, ok := s.byToken[token]
	if !ok {
		return ""
	}

	if login.SessionID != "" {
		// Login complete — clean up
		delete(s.pending, login.State)
		delete(s.byToken, token)
		return login.SessionID
	}

	return ""
}

// cleanupLocked removes expired pending logins (older than 10 minutes).
// Must be called with mu held.
func (s *CLILoginStore) cleanupLocked() {
	cutoff := time.Now().Add(-10 * time.Minute)
	for state, login := range s.pending {
		if login.CreatedAt.Before(cutoff) {
			delete(s.pending, state)
			delete(s.byToken, login.Token)
		}
	}
}

func generateToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("failed to generate token: " + err.Error())
	}
	return hex.EncodeToString(b)
}
