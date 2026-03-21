// Package appbase provides shared application infrastructure.
//
// Import this module to get database connections, authentication,
// HTTP server scaffolding, and CLI base for your application.
package appbase

import (
	"fmt"
	"net/http"

	"github.com/michaelwinser/appbase/auth"
	"github.com/michaelwinser/appbase/db"
	"github.com/michaelwinser/appbase/server"
)

// App is the central coordinator that manages connections and services.
type App struct {
	db       *db.DB
	sessions *auth.SessionStore
	google   *auth.GoogleAuth
	server   *server.Server
}

// Config configures an appbase application.
type Config struct {
	// GoogleAuth configures Google OAuth. Nil to use defaults.
	GoogleAuth *auth.GoogleAuthConfig
}

// New creates a new App with database, auth, and server initialized.
func New(config Config) (*App, error) {
	// Initialize database
	database, err := db.New()
	if err != nil {
		return nil, fmt.Errorf("initializing database: %w", err)
	}

	// Initialize sessions
	sessions, err := auth.NewSessionStore(database)
	if err != nil {
		database.Close()
		return nil, fmt.Errorf("initializing sessions: %w", err)
	}

	// Initialize Google Auth (nil if not configured)
	googleConfig := auth.GoogleAuthConfig{}
	if config.GoogleAuth != nil {
		googleConfig = *config.GoogleAuth
	}
	google := auth.NewGoogleAuth(sessions, googleConfig)

	// Initialize server
	srv := server.New()

	// Register auth middleware (must be before any routes)
	srv.Router().Use(auth.Middleware(sessions, nil))

	// Health endpoint
	srv.Router().Get("/health", func(w http.ResponseWriter, r *http.Request) {
		server.RespondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	app := &App{
		db:       database,
		sessions: sessions,
		google:   google,
		server:   srv,
	}

	// Register auth routes
	app.registerAuthRoutes()

	return app, nil
}

// DB returns the database connection.
func (a *App) DB() *db.DB {
	return a.db
}

// Sessions returns the session store.
func (a *App) Sessions() *auth.SessionStore {
	return a.sessions
}

// Google returns the Google OAuth handler (nil if not configured).
func (a *App) Google() *auth.GoogleAuth {
	return a.google
}

// Server returns the HTTP server.
func (a *App) Server() *server.Server {
	return a.server
}

// Router returns the chi router for registering routes.
func (a *App) Router() interface {
	http.Handler
	Get(string, http.HandlerFunc)
	Post(string, http.HandlerFunc)
	Put(string, http.HandlerFunc)
	Patch(string, http.HandlerFunc)
	Delete(string, http.HandlerFunc)
	Handle(string, http.Handler)
} {
	return a.server.Router()
}

// Serve starts the HTTP server. Blocks until exit.
func (a *App) Serve() error {
	return a.server.Serve()
}

// Close cleans up all resources.
func (a *App) Close() error {
	return a.db.Close()
}

// Migrate runs the application's SQL schema migration.
func (a *App) Migrate(schema string) error {
	return a.db.Migrate(schema)
}

// registerAuthRoutes sets up the standard auth endpoints.
func (a *App) registerAuthRoutes() {
	r := a.server.Router()

	// Auth status — works with or without session
	r.Get("/api/auth/status", func(w http.ResponseWriter, r *http.Request) {
		uid := auth.UserID(r)
		if uid == "" {
			server.RespondJSON(w, http.StatusOK, map[string]interface{}{
				"loggedIn": false,
			})
			return
		}
		server.RespondJSON(w, http.StatusOK, map[string]interface{}{
			"loggedIn": true,
			"email":    auth.Email(r),
		})
	})

	// Login URL
	r.Get("/api/auth/login", func(w http.ResponseWriter, r *http.Request) {
		if a.google == nil || !a.google.IsConfigured() {
			server.RespondError(w, http.StatusServiceUnavailable, "Google auth not configured")
			return
		}
		server.RespondJSON(w, http.StatusOK, map[string]string{
			"url": a.google.LoginURL(r),
		})
	})

	// OAuth callback
	r.Get("/api/auth/callback", func(w http.ResponseWriter, r *http.Request) {
		if a.google == nil {
			server.RespondError(w, http.StatusServiceUnavailable, "Google auth not configured")
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			server.RespondError(w, http.StatusBadRequest, "missing code parameter")
			return
		}

		result, err := a.google.HandleCallback(r, code)
		if err != nil {
			server.RespondError(w, http.StatusInternalServerError, err.Error())
			return
		}

		a.google.SetSessionCookie(w, r, result.Session.ID)
		server.RespondJSON(w, http.StatusOK, map[string]interface{}{
			"loggedIn": true,
			"email":    result.Email,
		})
	})

	// Logout
	r.Post("/api/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		if cookie, err := r.Cookie(auth.CookieName); err == nil && cookie.Value != "" {
			a.sessions.Delete(cookie.Value)
		}
		auth.ClearSessionCookie(w)
		server.RespondJSON(w, http.StatusOK, map[string]string{"ok": "true"})
	})
}

// UserID returns the authenticated user's ID from the request context.
// Convenience wrapper around auth.UserID.
func UserID(r *http.Request) string {
	return auth.UserID(r)
}

// Email returns the authenticated user's email from the request context.
// Convenience wrapper around auth.Email.
func Email(r *http.Request) string {
	return auth.Email(r)
}
