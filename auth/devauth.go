package auth

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"
)

// DevAuthMiddleware auto-authenticates requests when AUTH_MODE=dev.
// Creates a session for the configured dev email (DEV_USER_EMAIL or "dev@localhost")
// and sets the session cookie. Only active when AUTH_MODE=dev.
//
// This enables E2E testing without a browser OAuth flow.
// Restricted to APP_ENV=local by default for safety.
func DevAuthMiddleware(sessions *SessionStore) func(http.Handler) http.Handler {
	authMode := os.Getenv("AUTH_MODE")
	if authMode != "dev" {
		return func(next http.Handler) http.Handler { return next }
	}

	// Safety: only allow dev auth in local environment
	appEnv := os.Getenv("APP_ENV")
	if appEnv != "" && appEnv != "local" {
		log.Printf("WARNING: AUTH_MODE=dev ignored because APP_ENV=%s (only works in local)", appEnv)
		return func(next http.Handler) http.Handler { return next }
	}

	devEmail := os.Getenv("DEV_USER_EMAIL")
	if devEmail == "" {
		devEmail = "dev@localhost"
	}

	log.Printf("Dev auth mode active — auto-authenticating as %s", devEmail)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// If already authenticated, pass through
			if UserID(r) != "" {
				next.ServeHTTP(w, r)
				return
			}

			// Auto-create session and set cookie
			session, err := sessions.Create(devEmail, devEmail, 24*time.Hour)
			if err != nil {
				log.Printf("Dev auth: failed to create session: %v", err)
				next.ServeHTTP(w, r)
				return
			}

			http.SetCookie(w, &http.Cookie{
				Name:     CookieName,
				Value:    session.ID,
				Path:     "/",
				MaxAge:   24 * 60 * 60,
				HttpOnly: true,
			})

			// Populate context for this request
			ctx := context.WithValue(r.Context(), userIDKey, devEmail)
			ctx = context.WithValue(ctx, emailKey, devEmail)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
