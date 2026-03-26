package auth

import (
	"testing"
	"time"

	"github.com/michaelwinser/appbase/db"
)

func setupTestSessions(t *testing.T) *SessionStore {
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
	return sessions
}

func TestCreateAndGetSession(t *testing.T) {
	store := setupTestSessions(t)

	session, err := store.Create("user1", "user1@example.com", 1*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if session.ID == "" {
		t.Fatal("expected non-empty session ID")
	}
	if session.UserID != "user1" {
		t.Fatalf("expected user1, got %s", session.UserID)
	}

	// Retrieve
	got, err := store.Get(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("session not found")
	}
	if got.Email != "user1@example.com" {
		t.Fatalf("expected user1@example.com, got %s", got.Email)
	}
}

func TestGetSession_NotFound(t *testing.T) {
	store := setupTestSessions(t)

	got, err := store.Get("nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatal("expected nil for nonexistent session")
	}
}

func TestDeleteSession(t *testing.T) {
	store := setupTestSessions(t)

	session, _ := store.Create("user1", "user1@example.com", 1*time.Hour)
	if err := store.Delete(session.ID); err != nil {
		t.Fatal(err)
	}

	got, _ := store.Get(session.ID)
	if got != nil {
		t.Fatal("session should be deleted")
	}
}

func TestUpdateTokens(t *testing.T) {
	store := setupTestSessions(t)

	session, err := store.Create("user1", "user1@example.com", 1*time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	// Initially no tokens
	got, _ := store.Get(session.ID)
	if got.AccessToken != "" {
		t.Fatalf("expected empty access token, got %q", got.AccessToken)
	}

	// Store tokens
	expiry := time.Now().Add(1 * time.Hour).Truncate(time.Second)
	if err := store.UpdateTokens(session.ID, "access123", "refresh456", expiry); err != nil {
		t.Fatal(err)
	}

	// Verify tokens persisted
	got, err = store.Get(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.AccessToken != "access123" {
		t.Fatalf("expected access123, got %q", got.AccessToken)
	}
	if got.RefreshToken != "refresh456" {
		t.Fatalf("expected refresh456, got %q", got.RefreshToken)
	}
	if got.TokenExpiry.Unix() != expiry.Unix() {
		t.Fatalf("expected token expiry %v, got %v", expiry, got.TokenExpiry)
	}

	// Verify other fields unchanged
	if got.UserID != "user1" {
		t.Fatalf("UserID changed: got %q", got.UserID)
	}
	if got.Email != "user1@example.com" {
		t.Fatalf("Email changed: got %q", got.Email)
	}
}

func TestTokenExpired(t *testing.T) {
	s := &Session{}
	if !s.TokenExpired() {
		t.Error("zero TokenExpiry should be treated as expired")
	}

	s.TokenExpiry = time.Now().Add(1 * time.Hour)
	if s.TokenExpired() {
		t.Error("future TokenExpiry should not be expired")
	}

	s.TokenExpiry = time.Now().Add(-1 * time.Hour)
	if !s.TokenExpired() {
		t.Error("past TokenExpiry should be expired")
	}
}

func TestSessionExpiry(t *testing.T) {
	store := setupTestSessions(t)

	// Create already-expired session
	session, _ := store.Create("user1", "user1@example.com", -1*time.Hour)
	if !session.IsExpired() {
		t.Fatal("session should be expired")
	}

	// Cleanup should remove it
	if err := store.DeleteExpired(); err != nil {
		t.Fatal(err)
	}

	got, _ := store.Get(session.ID)
	if got != nil {
		t.Fatal("expired session should be deleted")
	}
}
