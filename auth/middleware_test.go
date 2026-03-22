package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/michaelwinser/appbase/db"
)

func setupTestMiddleware(t *testing.T) (*SessionStore, func(http.Handler) http.Handler) {
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

	mw := Middleware(sessions, nil)
	return sessions, mw
}

func TestMiddleware_AllowsExemptPaths(t *testing.T) {
	_, mw := setupTestMiddleware(t)

	tests := []struct {
		path string
		want int
	}{
		{"/api/auth/login", http.StatusOK},
		{"/api/auth/callback", http.StatusOK},
		{"/api/auth/status", http.StatusOK},
		{"/health", http.StatusOK},
		{"/", http.StatusOK},             // non-API paths exempt
		{"/index.html", http.StatusOK},   // non-API paths exempt
	}

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Errorf("path %s: got %d, want %d", tt.path, rec.Code, tt.want)
			}
		})
	}
}

func TestMiddleware_RejectsUnauthenticatedAPI(t *testing.T) {
	_, mw := setupTestMiddleware(t)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/todos", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("got %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestMiddleware_AllowsValidSession(t *testing.T) {
	sessions, mw := setupTestMiddleware(t)

	session, err := sessions.Create("user1", "user1@test.com", 1*time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	var gotUserID, gotEmail string
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserID = UserID(r)
		gotEmail = Email(r)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/todos", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: session.ID})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want %d", rec.Code, http.StatusOK)
	}
	if gotUserID != "user1" {
		t.Errorf("userID = %q, want %q", gotUserID, "user1")
	}
	if gotEmail != "user1@test.com" {
		t.Errorf("email = %q, want %q", gotEmail, "user1@test.com")
	}
}

func TestMiddleware_RejectsExpiredSession(t *testing.T) {
	sessions, mw := setupTestMiddleware(t)

	session, err := sessions.Create("user1", "user1@test.com", -1*time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/todos", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: session.ID})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("got %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	// Should have cleared the cookie
	cookies := rec.Result().Cookies()
	for _, c := range cookies {
		if c.Name == CookieName && c.MaxAge == -1 {
			return // cookie cleared correctly
		}
	}
	t.Error("expected session cookie to be cleared")
}

func TestMiddleware_PopulatesContextOnExemptPaths(t *testing.T) {
	sessions, mw := setupTestMiddleware(t)

	session, err := sessions.Create("user1", "user1@test.com", 1*time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	var gotUserID string
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserID = UserID(r)
		w.WriteHeader(http.StatusOK)
	}))

	// Even on exempt paths, context should be populated if cookie is valid
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: session.ID})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if gotUserID != "user1" {
		t.Errorf("userID = %q, want %q on exempt path with valid cookie", gotUserID, "user1")
	}
}

func TestMiddleware_InvalidSessionID(t *testing.T) {
	_, mw := setupTestMiddleware(t)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/todos", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: "bogus-session-id"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("got %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestWithIdentity(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	ctx := WithIdentity(req.Context(), "test-user", "test@example.com")
	req = req.WithContext(ctx)

	if got := UserID(req); got != "test-user" {
		t.Errorf("UserID = %q, want %q", got, "test-user")
	}
	if got := Email(req); got != "test@example.com" {
		t.Errorf("Email = %q, want %q", got, "test@example.com")
	}
}
