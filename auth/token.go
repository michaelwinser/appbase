package auth

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// TokenAuth handles static token authentication.
// Tokens map to email identities and create real sessions through the
// standard session pipeline (middleware, cookies, expiry).
//
// This provides a lightweight auth option for apps that don't need
// Google OAuth. Zero external dependencies — no GCP, no SMTP.
type TokenAuth struct {
	tokens       map[string]string // token -> email
	sessions     *SessionStore
	allowedUsers []string
}

// TokenAuthConfig configures static token authentication.
type TokenAuthConfig struct {
	// Tokens maps static tokens to email addresses.
	// e.g., {"dev-token-abc": "dev@localhost"}
	// Falls back to AUTH_TOKENS env var: "token1=email1,token2=email2"
	Tokens map[string]string
}

// NewTokenAuth creates a token auth handler.
// Returns nil if no tokens are configured (same pattern as NewGoogleAuth).
func NewTokenAuth(sessions *SessionStore, config TokenAuthConfig) *TokenAuth {
	tokens := make(map[string]string)
	for k, v := range config.Tokens {
		tokens[k] = v
	}

	// Fall back to AUTH_TOKENS env var
	if len(tokens) == 0 {
		if val := os.Getenv("AUTH_TOKENS"); val != "" {
			for _, pair := range strings.Split(val, ",") {
				pair = strings.TrimSpace(pair)
				if idx := strings.IndexByte(pair, '='); idx > 0 {
					token := strings.TrimSpace(pair[:idx])
					email := strings.TrimSpace(pair[idx+1:])
					if token != "" && email != "" {
						tokens[token] = email
					}
				}
			}
		}
	}

	if len(tokens) == 0 {
		return nil
	}

	// Validate minimum token length
	for token := range tokens {
		if len(token) < 8 {
			// Skip insecure tokens silently — don't expose which tokens exist
			delete(tokens, token)
		}
	}

	if len(tokens) == 0 {
		return nil
	}

	// Parse allowed users from env if not already set via GoogleAuth
	var allowedUsers []string
	if val := os.Getenv("ALLOWED_USERS"); val != "" {
		for _, u := range strings.Split(val, ",") {
			u = strings.TrimSpace(u)
			if u != "" {
				allowedUsers = append(allowedUsers, u)
			}
		}
	}

	return &TokenAuth{
		tokens:       tokens,
		sessions:     sessions,
		allowedUsers: allowedUsers,
	}
}

// IsConfigured returns true if token auth has at least one valid token.
func (t *TokenAuth) IsConfigured() bool {
	return t != nil && len(t.tokens) > 0
}

// HandleLogin validates a token and creates a session.
// Returns a LoginResult with a real session (same type as Google OAuth).
func (t *TokenAuth) HandleLogin(token string) (*LoginResult, error) {
	email, ok := t.tokens[token]
	if !ok {
		return nil, fmt.Errorf("invalid token")
	}

	// Check allowlist
	if len(t.allowedUsers) > 0 {
		allowed := false
		for _, u := range t.allowedUsers {
			if strings.EqualFold(u, email) {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, fmt.Errorf("user %s is not authorized", email)
		}
	}

	session, err := t.sessions.Create(email, email, 30*24*time.Hour)
	if err != nil {
		return nil, fmt.Errorf("creating session: %w", err)
	}

	return &LoginResult{
		Session: session,
		Email:   email,
	}, nil
}
