package auth

import (
	"fmt"
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

func TestMiddleware_SessionStoreError(t *testing.T) {
	// Simulate a session store that returns errors (e.g., SQLITE_BUSY).
	// Before the fix, errors were silently swallowed and the request
	// proceeded without context — the root cause of issue #22.
	errStore := &SessionStore{backend: &errorBackend{}}
	mw := Middleware(errStore, nil)

	var gotUserID string
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserID = UserID(r)
		w.WriteHeader(http.StatusOK)
	}))

	// On an exempt path, middleware should still call next even if session lookup fails
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: "some-session-id"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want %d (exempt path should still proceed)", rec.Code, http.StatusOK)
	}
	if gotUserID != "" {
		t.Errorf("userID = %q, want empty (session lookup failed)", gotUserID)
	}
}

// errorBackend is a sessionBackend that always returns errors from Get.
type errorBackend struct{}

func (e *errorBackend) Init() error                        { return nil }
func (e *errorBackend) Create(session *Session) error        { return nil }
func (e *errorBackend) Get(id string) (*Session, error)      { return nil, fmt.Errorf("database is locked") }
func (e *errorBackend) UpdateTokens(string, string, string, time.Time) error { return nil }
func (e *errorBackend) Delete(id string) error               { return nil }
func (e *errorBackend) DeleteExpired() error                 { return nil }
func (e *errorBackend) DeleteByUser(userID string) error     { return nil }

func TestMiddleware_PopulatesTokenContext(t *testing.T) {
	sessions, mw := setupTestMiddleware(t)

	session, err := sessions.Create("user1", "user1@test.com", 1*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	expiry := time.Now().Add(1 * time.Hour)
	if err := sessions.UpdateTokens(session.ID, "tok_abc", "ref_xyz", expiry); err != nil {
		t.Fatal(err)
	}

	var gotToken, gotRefresh string
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = AccessToken(r)
		gotRefresh = RefreshToken(r)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/todos", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: session.ID})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if gotToken != "tok_abc" {
		t.Errorf("AccessToken = %q, want %q", gotToken, "tok_abc")
	}
	if gotRefresh != "ref_xyz" {
		t.Errorf("RefreshToken = %q, want %q", gotRefresh, "ref_xyz")
	}
}

func TestMiddleware_TestModeHeader(t *testing.T) {
	t.Setenv("APPBASE_TEST_MODE", "true")

	_, mw := setupTestMiddleware(t)

	var gotUserID string
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserID = UserID(r)
		w.WriteHeader(http.StatusOK)
	}))

	// X-Test-User should authenticate on protected API paths
	req := httptest.NewRequest("GET", "/api/todos", nil)
	req.Header.Set("X-Test-User", "testuser@example.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want %d", rec.Code, http.StatusOK)
	}
	if gotUserID != "testuser@example.com" {
		t.Errorf("userID = %q, want %q", gotUserID, "testuser@example.com")
	}
}

func TestMiddleware_TestModeDisabled(t *testing.T) {
	// Ensure test mode is off
	t.Setenv("APPBASE_TEST_MODE", "")

	_, mw := setupTestMiddleware(t)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// X-Test-User should be ignored when test mode is off
	req := httptest.NewRequest("GET", "/api/todos", nil)
	req.Header.Set("X-Test-User", "testuser@example.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("got %d, want %d (test mode off, header should be ignored)", rec.Code, http.StatusUnauthorized)
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
