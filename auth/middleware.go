package auth

import (
	"context"
	"log"
	"net/http"
	"strings"
)

type contextKey string

const (
	userIDKey  contextKey = "appbase_userID"
	emailKey   contextKey = "appbase_email"
	CookieName            = "app_session"
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

// Middleware returns HTTP middleware that enforces session authentication.
// It always populates user context from the session cookie if valid.
// For non-exempt API paths, it rejects requests without a valid session.
func Middleware(sessions *SessionStore, exemptPrefixes []string) func(http.Handler) http.Handler {
	if exemptPrefixes == nil {
		exemptPrefixes = []string{"/api/auth/", "/health"}
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
			// Always try to populate context from session cookie
			if cookie, err := r.Cookie(CookieName); err == nil && cookie.Value != "" {
				session, err := sessions.Get(cookie.Value)
				if err != nil {
					log.Printf("auth: session lookup error for cookie %s…: %v", cookie.Value[:min(8, len(cookie.Value))], err)
				} else if session == nil {
					log.Printf("auth: session not found for cookie %s… (no error)", cookie.Value[:min(8, len(cookie.Value))])
				} else if session.IsExpired() {
					log.Printf("auth: session expired for %s", session.Email)
					sessions.Delete(session.ID)
					http.SetCookie(w, &http.Cookie{
						Name: CookieName, Value: "", Path: "/",
						MaxAge: -1, HttpOnly: true,
					})
				} else {
					ctx := context.WithValue(r.Context(), userIDKey, session.UserID)
					ctx = context.WithValue(ctx, emailKey, session.Email)
					r = r.WithContext(ctx)
				}
			} else if err != nil {
				log.Printf("auth: no %s cookie on %s %s", CookieName, r.Method, r.URL.Path)
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
