package auth

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// roundTripFunc lets a func act as an http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// fakeTokenResponse builds a RoundTripper that returns the given JSON body
// for any request, and records the form values it received.
func fakeTokenResponse(status int, body string, sentForm *url.Values) http.RoundTripper {
	return roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if sentForm != nil {
			b, _ := io.ReadAll(r.Body)
			*sentForm, _ = url.ParseQuery(string(b))
		}
		return &http.Response{
			StatusCode: status,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	})
}

// newTestGoogleAuth builds a GoogleAuth with a fake HTTP transport and a
// real in-memory session store.
func newTestGoogleAuth(t *testing.T, transport http.RoundTripper) (*GoogleAuth, *SessionStore) {
	t.Helper()
	sessions := setupTestSessions(t)
	g := &GoogleAuth{
		clientID:     "test-client",
		clientSecret: "test-secret",
		sessions:     sessions,
		httpClient:   &http.Client{Transport: transport},
	}
	return g, sessions
}

// --- ExchangeRefreshToken ---

func TestExchangeRefreshToken_Success(t *testing.T) {
	var sent url.Values
	g, _ := newTestGoogleAuth(t, fakeTokenResponse(200,
		`{"access_token":"new-access","expires_in":3600}`, &sent))

	before := time.Now()
	access, refresh, expiry, err := g.ExchangeRefreshToken(context.Background(), "old-refresh")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if access != "new-access" {
		t.Errorf("access = %q, want %q", access, "new-access")
	}
	if refresh != "old-refresh" {
		t.Errorf("refresh = %q, want %q (preserved when Google omits it)", refresh, "old-refresh")
	}
	if expiry.Before(before.Add(3590*time.Second)) || expiry.After(time.Now().Add(3610*time.Second)) {
		t.Errorf("expiry = %v, want ~now+3600s", expiry)
	}

	// Verify the request was well-formed
	if sent.Get("grant_type") != "refresh_token" {
		t.Errorf("grant_type = %q, want refresh_token", sent.Get("grant_type"))
	}
	if sent.Get("refresh_token") != "old-refresh" {
		t.Errorf("sent refresh_token = %q, want old-refresh", sent.Get("refresh_token"))
	}
	if sent.Get("client_id") != "test-client" {
		t.Errorf("client_id = %q, want test-client", sent.Get("client_id"))
	}
}

func TestExchangeRefreshToken_RotatedRefresh(t *testing.T) {
	g, _ := newTestGoogleAuth(t, fakeTokenResponse(200,
		`{"access_token":"new-access","refresh_token":"rotated-refresh","expires_in":3600}`, nil))

	_, refresh, _, err := g.ExchangeRefreshToken(context.Background(), "old-refresh")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if refresh != "rotated-refresh" {
		t.Errorf("refresh = %q, want rotated-refresh (Google sent a new one)", refresh)
	}
}

func TestExchangeRefreshToken_EmptyToken(t *testing.T) {
	g, _ := newTestGoogleAuth(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("HTTP client should not be called with empty refresh token")
		return nil, nil
	}))

	_, _, _, err := g.ExchangeRefreshToken(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty refresh token")
	}
}

func TestExchangeRefreshToken_HTTPError(t *testing.T) {
	g, _ := newTestGoogleAuth(t, fakeTokenResponse(400,
		`{"error":"invalid_grant","error_description":"Token has been revoked"}`, nil))

	_, _, _, err := g.ExchangeRefreshToken(context.Background(), "revoked")
	if err == nil {
		t.Fatal("expected error for non-200 response")
	}
	if !strings.Contains(err.Error(), "invalid_grant") {
		t.Errorf("error %q does not contain server response body", err)
	}
}

// --- RefreshAccessToken ---

func TestRefreshAccessToken_PersistedSession(t *testing.T) {
	g, sessions := newTestGoogleAuth(t, fakeTokenResponse(200,
		`{"access_token":"fresh-access","refresh_token":"fresh-refresh","expires_in":3600}`, nil))

	session, err := sessions.Create("user1", "user1@example.com", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	session.RefreshToken = "initial-refresh"

	access, err := g.RefreshAccessToken(context.Background(), session)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if access != "fresh-access" {
		t.Errorf("returned access = %q, want fresh-access", access)
	}

	// In-memory struct updated
	if session.AccessToken != "fresh-access" {
		t.Errorf("session.AccessToken = %q, want fresh-access", session.AccessToken)
	}
	if session.RefreshToken != "fresh-refresh" {
		t.Errorf("session.RefreshToken = %q, want fresh-refresh", session.RefreshToken)
	}

	// Store updated — reload and verify
	reloaded, err := sessions.Get(session.ID)
	if err != nil {
		t.Fatalf("reloading session: %v", err)
	}
	if reloaded.AccessToken != "fresh-access" {
		t.Errorf("persisted AccessToken = %q, want fresh-access", reloaded.AccessToken)
	}
	if reloaded.RefreshToken != "fresh-refresh" {
		t.Errorf("persisted RefreshToken = %q, want fresh-refresh", reloaded.RefreshToken)
	}
}

// TestRefreshAccessToken_TransientSession is the regression test for #49:
// a session with an empty ID must not attempt a store write. The Firestore
// backend rejects Doc("") with InvalidArgument; we simulate that with a
// backend that errors on UpdateTokens("").
func TestRefreshAccessToken_TransientSession(t *testing.T) {
	g, _ := newTestGoogleAuth(t, fakeTokenResponse(200,
		`{"access_token":"bg-access","expires_in":3600}`, nil))

	// Replace the store with one that fails on empty-ID writes (Firestore behavior).
	g.sessions = &SessionStore{backend: &emptyIDRejectingBackend{}}

	// The background-job pattern: just a refresh token, no session ID.
	session := &Session{RefreshToken: "bg-refresh"}

	access, err := g.RefreshAccessToken(context.Background(), session)
	if err != nil {
		t.Fatalf("transient session should not fail on store write: %v", err)
	}
	if access != "bg-access" {
		t.Errorf("access = %q, want bg-access", access)
	}

	// In-memory struct still updated so the caller can persist it themselves.
	if session.AccessToken != "bg-access" {
		t.Errorf("session.AccessToken = %q, want bg-access", session.AccessToken)
	}
	if session.RefreshToken != "bg-refresh" {
		t.Errorf("session.RefreshToken = %q, want bg-refresh (preserved)", session.RefreshToken)
	}
	if session.TokenExpiry.IsZero() {
		t.Error("session.TokenExpiry should be set")
	}
}

func TestRefreshAccessToken_StoreErrorPropagates(t *testing.T) {
	// With a real session ID, store errors must still surface.
	g, _ := newTestGoogleAuth(t, fakeTokenResponse(200,
		`{"access_token":"x","expires_in":3600}`, nil))
	g.sessions = &SessionStore{backend: &alwaysFailBackend{}}

	session := &Session{ID: "real-id", RefreshToken: "tok"}

	_, err := g.RefreshAccessToken(context.Background(), session)
	if err == nil {
		t.Fatal("expected store error to propagate when session.ID is set")
	}
	if !strings.Contains(err.Error(), "storing refreshed token") {
		t.Errorf("error %q missing context wrap", err)
	}
}

// --- test backends ---

// emptyIDRejectingBackend mimics Firestore's behavior: Doc("") produces
// an InvalidArgument because the document path has a trailing slash.
type emptyIDRejectingBackend struct{}

func (*emptyIDRejectingBackend) Init() error                   { return nil }
func (*emptyIDRejectingBackend) Create(*Session) error         { return nil }
func (*emptyIDRejectingBackend) Get(string) (*Session, error)  { return nil, nil }
func (*emptyIDRejectingBackend) Delete(string) error           { return nil }
func (*emptyIDRejectingBackend) DeleteExpired() error          { return nil }
func (*emptyIDRejectingBackend) DeleteByUser(string) error     { return nil }
func (*emptyIDRejectingBackend) UpdateTokens(id, _, _ string, _ time.Time) error {
	if id == "" {
		return fmt.Errorf(`rpc error: code = InvalidArgument desc = Document name "projects/p/databases/(default)/documents/sessions/" has invalid trailing "/"`)
	}
	return nil
}

// alwaysFailBackend errors on every UpdateTokens call.
type alwaysFailBackend struct{}

func (*alwaysFailBackend) Init() error                   { return nil }
func (*alwaysFailBackend) Create(*Session) error         { return nil }
func (*alwaysFailBackend) Get(string) (*Session, error)  { return nil, nil }
func (*alwaysFailBackend) Delete(string) error           { return nil }
func (*alwaysFailBackend) DeleteExpired() error          { return nil }
func (*alwaysFailBackend) DeleteByUser(string) error     { return nil }
func (*alwaysFailBackend) UpdateTokens(string, string, string, time.Time) error {
	return fmt.Errorf("database is locked")
}
