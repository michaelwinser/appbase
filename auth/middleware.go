package auth

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type contextKey string

const (
	userIDKey       contextKey = "appbase_userID"
	emailKey        contextKey = "appbase_email"
	accessTokenKey  contextKey = "appbase_accessToken"
	refreshTokenKey contextKey = "appbase_refreshToken"
	tokenExpiryKey  contextKey = "appbase_tokenExpiry"
	CookieName                 = "app_session"
)

// WithIdentity returns a context with the given user identity set.
// Used by in-process transports to inject identity without sessions.
func WithIdentity(ctx context.Context, userID, email string) context.Context {
	ctx = context.WithValue(ctx, userIDKey, userID)
	ctx = context.WithValue(ctx, emailKey, email)
	return ctx
}

// UserID returns the authenticated user's ID from the request context.
func UserID(r *http.Request) string {
	if v, ok := r.Context().Value(userIDKey).(string); ok {
		return v
	}
	return ""
}

// Email returns the authenticated user's email from the request context.
func Email(r *http.Request) string {
	if v, ok := r.Context().Value(emailKey).(string); ok {
		return v
	}
	return ""
}

// AccessToken returns the OAuth access token from the request context.
// Returns empty string if no token is stored (e.g., local mode sessions).
func AccessToken(r *http.Request) string {
	if v, ok := r.Context().Value(accessTokenKey).(string); ok {
		return v
	}
	return ""
}

// RefreshToken returns the OAuth refresh token from the request context.
func RefreshToken(r *http.Request) string {
	if v, ok := r.Context().Value(refreshTokenKey).(string); ok {
		return v
	}
	return ""
}

// TokenExpiry returns the OAuth access token expiry time from the request context.
func TokenExpiry(r *http.Request) time.Time {
	if v, ok := r.Context().Value(tokenExpiryKey).(time.Time); ok {
		return v
	}
	return time.Time{}
}

// TestMode returns true when APPBASE_TEST_MODE=true, enabling X-Test-User
// header authentication for CI and API-level testing.
func TestMode() bool {
	return os.Getenv("APPBASE_TEST_MODE") == "true"
}

// Middleware returns HTTP middleware that enforces session authentication.
// It always populates user context from the session cookie if valid.
// For non-exempt API paths, it rejects requests without a valid session.
//
// When APPBASE_TEST_MODE=true, also accepts X-Test-User header as identity.
func Middleware(sessions *SessionStore, exemptPrefixes []string) func(http.Handler) http.Handler {
	if exemptPrefixes == nil {
		exemptPrefixes = []string{"/api/auth/", "/health"}
	}

	testMode := TestMode()
	if testMode {
		log.Println("WARNING: test authentication enabled (APPBASE_TEST_MODE=true)")
	}

	isExempt := func(path string) bool {
		for _, prefix := range exemptPrefixes {
			if strings.HasPrefix(path, prefix) {
				return true
			}
		}
		if !strings.HasPrefix(path, "/api/") {
			return true
		}
		return false
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Test mode: accept X-Test-User header as identity
			if testMode {
				if testUser := r.Header.Get("X-Test-User"); testUser != "" {
					ctx := WithIdentity(r.Context(), testUser, testUser)
					r = r.WithContext(ctx)
					next.ServeHTTP(w, r)
					return
				}
			}

			// Always try to populate context from session cookie
			if cookie, err := r.Cookie(CookieName); err == nil && cookie.Value != "" {
				session, err := sessions.Get(cookie.Value)
				if err != nil {
					log.Printf("auth: session lookup error for cookie %s…: %v", cookie.Value[:min(8, len(cookie.Value))], err)
				} else if session != nil && session.IsExpired() {
					sessions.Delete(session.ID)
					http.SetCookie(w, &http.Cookie{
						Name: CookieName, Value: "", Path: "/",
						MaxAge: -1, HttpOnly: true,
					})
				} else if session != nil {
					ctx := context.WithValue(r.Context(), userIDKey, session.UserID)
					ctx = context.WithValue(ctx, emailKey, session.Email)
					ctx = context.WithValue(ctx, accessTokenKey, session.AccessToken)
					ctx = context.WithValue(ctx, refreshTokenKey, session.RefreshToken)
					ctx = context.WithValue(ctx, tokenExpiryKey, session.TokenExpiry)
					r = r.WithContext(ctx)
				}
			}

			if isExempt(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			if UserID(r) == "" {
				w.Header().Set("Content-Type", "application/json")
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
