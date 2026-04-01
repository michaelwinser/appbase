package auth

import (
	"testing"

	"github.com/michaelwinser/appbase/db"
)

func setupTestTokenAuth(t *testing.T, tokens map[string]string) (*TokenAuth, *SessionStore) {
	t.Helper()
	database, err := db.New(db.DBConfig{StoreType: "sqlite", SQLitePath: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })

	sessions, err := NewSessionStore(database)
	if err != nil {
		t.Fatal(err)
	}

	ta := NewTokenAuth(sessions, TokenAuthConfig{Tokens: tokens})
	return ta, sessions
}

func TestTokenAuth_HandleLogin(t *testing.T) {
	ta, sessions := setupTestTokenAuth(t, map[string]string{
		"valid-token-12345": "user@example.com",
	})

	result, err := ta.HandleLogin("valid-token-12345")
	if err != nil {
		t.Fatal(err)
	}
	if result.Email != "user@example.com" {
		t.Fatalf("expected user@example.com, got %s", result.Email)
	}
	if result.Session == nil {
		t.Fatal("expected session")
	}
	if result.Session.ID == "" {
		t.Fatal("expected session ID")
	}

	// Session should be retrievable from the store
	got, err := sessions.Get(result.Session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("session not found in store")
	}
	if got.Email != "user@example.com" {
		t.Fatalf("session email = %q, want %q", got.Email, "user@example.com")
	}
}

func TestTokenAuth_InvalidToken(t *testing.T) {
	ta, _ := setupTestTokenAuth(t, map[string]string{
		"valid-token-12345": "user@example.com",
	})

	_, err := ta.HandleLogin("wrong-token-xyz")
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
}

func TestTokenAuth_ShortTokenRejected(t *testing.T) {
	ta, _ := setupTestTokenAuth(t, map[string]string{
		"short": "user@example.com", // < 8 chars, should be rejected
	})

	if ta != nil {
		t.Fatal("expected nil TokenAuth for short token")
	}
}

func TestTokenAuth_NilIsNotConfigured(t *testing.T) {
	ta := NewTokenAuth(nil, TokenAuthConfig{})
	if ta != nil {
		t.Fatal("expected nil for empty config")
	}
	// IsConfigured on nil should be safe
	var nilTA *TokenAuth
	if nilTA.IsConfigured() {
		t.Fatal("nil TokenAuth should not be configured")
	}
}

func TestTokenAuth_AllowedUsersEnforced(t *testing.T) {
	database, err := db.New(db.DBConfig{StoreType: "sqlite", SQLitePath: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })

	sessions, err := NewSessionStore(database)
	if err != nil {
		t.Fatal(err)
	}

	ta := &TokenAuth{
		tokens:       map[string]string{"valid-token-12345": "blocked@example.com"},
		sessions:     sessions,
		allowedUsers: []string{"allowed@example.com"},
	}

	_, err = ta.HandleLogin("valid-token-12345")
	if err == nil {
		t.Fatal("expected error for user not in allowlist")
	}
}

func TestTokenAuth_EnvVar(t *testing.T) {
	t.Setenv("AUTH_TOKENS", "mytoken123=dev@localhost,citoken99=ci@test.com")

	database, err := db.New(db.DBConfig{StoreType: "sqlite", SQLitePath: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })

	sessions, err := NewSessionStore(database)
	if err != nil {
		t.Fatal(err)
	}

	ta := NewTokenAuth(sessions, TokenAuthConfig{})
	if ta == nil {
		t.Fatal("expected TokenAuth from env var")
	}

	result, err := ta.HandleLogin("mytoken123")
	if err != nil {
		t.Fatal(err)
	}
	if result.Email != "dev@localhost" {
		t.Fatalf("expected dev@localhost, got %s", result.Email)
	}
}
