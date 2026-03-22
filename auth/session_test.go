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
