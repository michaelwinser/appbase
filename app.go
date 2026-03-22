// Package appbase provides shared application infrastructure.
//
// Import this module to get database connections, authentication,
// HTTP server scaffolding, and CLI base for your application.
package appbase

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/michaelwinser/appbase/auth"
	appconfig "github.com/michaelwinser/appbase/config"
	"github.com/michaelwinser/appbase/db"
	"github.com/michaelwinser/appbase/server"
)

// App is the central coordinator that manages connections and services.
type App struct {
	db        *db.DB
	sessions  *auth.SessionStore
	google    *auth.GoogleAuth
	server    *server.Server
	cliLogins *auth.CLILoginStore
	name      string
}

// Config configures an appbase application.
type Config struct {
	// Name is the application name, shown on the login page.
	Name string

	// GoogleAuth configures Google OAuth. Nil to use defaults.
	GoogleAuth *auth.GoogleAuthConfig
}

// New creates a new App with database, auth, and server initialized.
// If app.yaml exists, loads it and exports config as env vars so that
// db.New(), auth, and server pick up the settings automatically.
func New(config Config) (*App, error) {
	// Load app.yaml if present — sets env vars for downstream components
	configPath := "app.yaml"
	if _, err := os.Stat(configPath); err == nil {
		var secrets appconfig.SecretResolver
		// Use the default resolver chain (keychain → docker → .env → GCP)
		gcpProject := os.Getenv("GOOGLE_CLOUD_PROJECT")
		if gcpProject != "" {
			secrets = appconfig.DefaultResolver(gcpProject)
		} else {
			// Minimal resolver without GCP (local dev)
			secrets = appconfig.NewChainResolver(
				&appconfig.KeychainResolver{},
				&appconfig.DockerSecretResolver{},
				&appconfig.EnvFileResolver{},
			)
		}
		appCfg, err := appconfig.LoadAppConfig(configPath, secrets)
		if err != nil {
			log.Printf("Warning: could not load app.yaml: %v", err)
		} else {
			appCfg.SetEnvVars()
			if config.Name == "" {
				config.Name = appCfg.Name
			}
			log.Printf("Loaded config from app.yaml (env: %s)", appCfg.Env())
		}
	}

	// Initialize database
	database, err := db.New()
	if err != nil {
		return nil, fmt.Errorf("initializing database: %w", err)
	}

	// Preflight check — verify the backend is functional
	if err := database.Preflight(); err != nil {
		database.Close()
		return nil, fmt.Errorf("database preflight failed: %w", err)
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

	name := config.Name
	if name == "" {
		name = "App"
	}

	app := &App{
		db:        database,
		sessions:  sessions,
		google:    google,
		server:    srv,
		cliLogins: auth.NewCLILoginStore(),
		name:      name,
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

	// OAuth callback — handles both browser and CLI login flows.
	// CLI logins use a state prefixed with "cli:" to identify the pending login.
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

		// Check if this is a CLI login (state starts with "cli:")
		state := r.URL.Query().Get("state")
		if strings.HasPrefix(state, "cli:") {
			a.cliLogins.Complete(state, result.Session.ID)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte(`<!DOCTYPE html>
<html><head><title>Login Successful</title></head>
<body style="font-family:system-ui;text-align:center;padding:3rem">
<h1>Login successful</h1>
<p>You can close this tab and return to your terminal.</p>
</body></html>`))
			return
		}

		a.google.SetSessionCookie(w, r, result.Session.ID)
		http.Redirect(w, r, "/", http.StatusFound)
	})

	// CLI login — initiate a CLI login flow.
	// Returns a login URL and polling token.
	r.Post("/api/auth/cli-login", func(w http.ResponseWriter, r *http.Request) {
		if a.google == nil || !a.google.IsConfigured() {
			server.RespondError(w, http.StatusServiceUnavailable, "Google auth not configured")
			return
		}
		token, state := a.cliLogins.Create()
		loginURL := a.google.LoginURLWithState(r, state)
		server.RespondJSON(w, http.StatusOK, map[string]string{
			"loginURL": loginURL,
			"token":    token,
		})
	})

	// CLI poll — check if a CLI login has completed.
	r.Get("/api/auth/cli-poll", func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("token")
		if token == "" {
			server.RespondError(w, http.StatusBadRequest, "missing token parameter")
			return
		}
		sessionID := a.cliLogins.Poll(token)
		if sessionID == "" {
			server.RespondJSON(w, http.StatusOK, map[string]interface{}{
				"completed": false,
			})
			return
		}
		session, err := a.sessions.Get(sessionID)
		if err != nil || session == nil {
			server.RespondError(w, http.StatusInternalServerError, "session not found")
			return
		}
		server.RespondJSON(w, http.StatusOK, map[string]interface{}{
			"completed": true,
			"sessionID": sessionID,
			"email":     session.Email,
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

// LoginPage returns an HTTP handler that shows a login page when the user
// is not authenticated, and calls next when they are. If next is nil,
// a default welcome page is shown for authenticated users.
func (a *App) LoginPage(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if auth.UserID(r) == "" {
			auth.ServeLoginPage(w, r, a.name, a.google)
			return
		}
		if next != nil {
			next(w, r)
			return
		}
		// Default authenticated page
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<!DOCTYPE html>
<html><head><title>%s</title></head>
<body style="font-family:system-ui;max-width:600px;margin:2rem auto;padding:0 1rem">
<h1>%s</h1>
<p>Signed in as %s</p>
<form method="POST" action="/api/auth/logout"><button>Sign out</button></form>
</body></html>`, a.name, a.name, auth.Email(r))
	}
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
