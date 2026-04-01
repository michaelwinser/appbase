// Package appbase provides shared application infrastructure.
//
// Import this module to get database connections, authentication,
// HTTP server scaffolding, and CLI base for your application.
package appbase

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"

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
	tokenAuth *auth.TokenAuth
	server    *server.Server
	cliLogins *auth.CLILoginStore
	name      string
	localMode bool
}

// Config configures an appbase application.
type Config struct {
	// Name is the application name, shown on the login page.
	Name string

	// GoogleAuth configures Google OAuth. Nil to use defaults.
	GoogleAuth *auth.GoogleAuthConfig

	// TokenAuth configures static token authentication.
	// Provides a lightweight auth option that works without any third-party
	// setup (no GCP, no SMTP). Creates real sessions through the standard
	// session pipeline. Nil to use defaults from app.yaml or AUTH_TOKENS env.
	TokenAuth *auth.TokenAuthConfig

	// Quiet suppresses startup log messages (config loading, preflight, etc.).
	// Useful for CLI commands where log noise is unwanted.
	Quiet bool

	// LocalMode configures the app for local/desktop use without OAuth.
	// When true:
	//   - DevAuth middleware is skipped (identity injected at transport layer)
	//   - /api/auth/status always returns logged-in
	//   - DB path defaults to ~/.config/<name>/app.db if not set
	// Use this for Wails desktop apps or embedded contexts.
	LocalMode bool

	// AllowedOrigins for CORS on API routes. If empty, no CORS headers are set
	// (same-origin only). Set to ["*"] for public APIs without authentication.
	// For authenticated apps, list specific origins (e.g., ["http://localhost:3000"]).
	AllowedOrigins []string

	// Port for the HTTP server. Falls back to PORT env var, then "3000".
	Port string

	// DB configures the database connection. Falls back to env vars if not set.
	DB db.DBConfig
}

// New creates a new App with database, auth, and server initialized.
// If app.yaml exists, loads it and merges config values (explicit Config
// fields take precedence over app.yaml, which takes precedence over env vars).
func New(config Config) (*App, error) {
	// Suppress log output for CLI commands
	if config.Quiet {
		log.SetOutput(io.Discard)
		defer log.SetOutput(os.Stderr)
	}

	// LocalMode: default DB path to ~/.config/<name>/app.db
	if config.LocalMode && config.DB.SQLitePath == "" {
		home, _ := os.UserHomeDir()
		if home != "" && config.Name != "" {
			dataDir := home + "/.config/" + config.Name
			os.MkdirAll(dataDir, 0755)
			config.DB.SQLitePath = dataDir + "/app.db"
		}
	}

	// Load app.yaml if present — merge into config (explicit fields win)
	var appCfg *appconfig.AppConfig
	configPath := "app.yaml"
	if _, err := os.Stat(configPath); err == nil {
		gcpProject := config.DB.GCPProject
		if gcpProject == "" {
			gcpProject = os.Getenv("GOOGLE_CLOUD_PROJECT")
		}
		var secrets appconfig.SecretResolver
		if gcpProject != "" {
			secrets = appconfig.DefaultResolver(gcpProject)
		} else {
			secrets = appconfig.NewChainResolver(
				&appconfig.KeychainResolver{},
				&appconfig.DockerSecretResolver{},
				&appconfig.EnvFileResolver{},
			)
		}
		var err error
		appCfg, err = appconfig.LoadAppConfig(configPath, secrets)
		if err != nil {
			log.Printf("Warning: could not load app.yaml: %v", err)
		} else {
			// Merge app.yaml values into config (explicit Config fields take precedence)
			if config.Name == "" {
				config.Name = appCfg.Name
			}
			if config.Port == "" && appCfg.Port != 0 {
				config.Port = fmt.Sprintf("%d", appCfg.Port)
			}
			if config.DB.StoreType == "" {
				config.DB.StoreType = appCfg.Store.Type
			}
			if config.DB.SQLitePath == "" {
				config.DB.SQLitePath = appCfg.Store.Path
			}
			if config.DB.GCPProject == "" {
				config.DB.GCPProject = appCfg.Store.GCPProject
			}
			// Merge YAML auth config with any app-provided GoogleAuth.
			// App-set fields take precedence; empty fields fall back to YAML.
			// ExtraScopes are combined from both sources.
			if appCfg.Auth.ClientID != "" || appCfg.Auth.ClientSecret != "" || len(appCfg.Auth.ExtraScopes) > 0 {
				if config.GoogleAuth == nil {
					config.GoogleAuth = &auth.GoogleAuthConfig{}
				}
				if config.GoogleAuth.ClientID == "" {
					config.GoogleAuth.ClientID = appCfg.Auth.ClientID
				}
				if config.GoogleAuth.ClientSecret == "" {
					config.GoogleAuth.ClientSecret = appCfg.Auth.ClientSecret
				}
				if config.GoogleAuth.RedirectURL == "" {
					config.GoogleAuth.RedirectURL = appCfg.Auth.RedirectURL
				}
				if len(config.GoogleAuth.AllowedUsers) == 0 {
					config.GoogleAuth.AllowedUsers = appCfg.Auth.AllowedUsers
				}
				// Append YAML scopes to any app-provided scopes
				config.GoogleAuth.ExtraScopes = append(config.GoogleAuth.ExtraScopes, appCfg.Auth.ExtraScopes...)
			}
			// Merge token auth from YAML
			if len(appCfg.Auth.Tokens) > 0 {
				if config.TokenAuth == nil {
					config.TokenAuth = &auth.TokenAuthConfig{}
				}
				if len(config.TokenAuth.Tokens) == 0 {
					config.TokenAuth.Tokens = appCfg.Auth.Tokens
				}
			}
			log.Printf("Loaded config from app.yaml (env: %s)", appCfg.Env())
		}
	}

	// Initialize database with explicit config
	database, err := db.New(config.DB)
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

	// Initialize Token Auth (nil if not configured)
	tokenConfig := auth.TokenAuthConfig{}
	if config.TokenAuth != nil {
		tokenConfig = *config.TokenAuth
	}
	tokenAuth := auth.NewTokenAuth(sessions, tokenConfig)

	// Initialize server
	srv := server.New(server.Config{
		Port:           config.Port,
		AllowedOrigins: config.AllowedOrigins,
		Quiet:          config.Quiet,
	})

	// Register auth middleware (must be before any routes)
	if config.LocalMode {
		// LocalMode: no DevAuth, no session middleware.
		// Identity is injected at the transport layer:
		//   - CLI: handlerTransport injects identity per-request
		//   - Desktop: LocalHandler() wraps the handler with identity injection
	} else {
		// DevAuth runs first — populates context if AUTH_MODE=dev (no-op otherwise)
		// Regular auth runs second — sees the populated context and passes through
		srv.Router().Use(auth.DevAuthMiddleware(sessions))
		srv.Router().Use(auth.Middleware(sessions, nil, google))
	}

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
		tokenAuth: tokenAuth,
		server:    srv,
		cliLogins: auth.NewCLILoginStore(),
		name:      name,
		localMode: config.LocalMode,
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

// Handler returns the HTTP handler for the app.
// For desktop/embedded use with LocalMode, use LocalHandler() instead.
func (a *App) Handler() http.Handler {
	return a.server.Router()
}

// LocalHandler returns an http.Handler that injects local user identity
// into every request at the transport boundary. Use this for Wails desktop
// integration or any in-process context where there is no handlerTransport.
//
//	wails.Run(&options.App{
//	    AssetServer: &assetserver.Options{Handler: app.LocalHandler()},
//	})
func (a *App) LocalHandler() http.Handler {
	email := "dev@localhost"
	if e := os.Getenv("DEV_USER_EMAIL"); e != "" {
		email = e
	}
	handler := a.server.Router()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := auth.WithIdentity(r.Context(), email, email)
		handler.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Router returns the chi router for registering routes.
// Returns chi.Router directly so callers can use Route(), Group(), Mount(),
// With(), and other chi features without type assertions.
func (a *App) Router() chi.Router {
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
		// LocalMode: always report as logged in
		if a.localMode {
			email := auth.Email(r)
			if email == "" {
				email = "dev@localhost"
			}
			server.RespondJSON(w, http.StatusOK, map[string]interface{}{
				"loggedIn": true,
				"email":    email,
			})
			return
		}

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

	// Login URL — browser navigation redirects to Google; fetch returns JSON.
	r.Get("/api/auth/login", func(w http.ResponseWriter, r *http.Request) {
		if a.google == nil || !a.google.IsConfigured() {
			server.RespondError(w, http.StatusServiceUnavailable, "Google auth not configured")
			return
		}
		loginURL := a.google.LoginURL(w, r)

		// Browser navigation (e.g. login page link): redirect to Google directly.
		// This ensures the state cookie is set in the same response as the redirect,
		// avoiding races with concurrent requests (favicon, etc.).
		if strings.Contains(r.Header.Get("Accept"), "text/html") {
			http.Redirect(w, r, loginURL, http.StatusFound)
			return
		}

		// SPA / fetch: return JSON with the URL.
		server.RespondJSON(w, http.StatusOK, map[string]string{
			"url": loginURL,
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

		// Validate state parameter (CSRF protection)
		state := r.URL.Query().Get("state")
		if err := a.google.ValidateState(w, r, state); err != nil {
			server.RespondError(w, http.StatusForbidden, err.Error())
			return
		}

		result, err := a.google.HandleCallback(r, code)
		if err != nil {
			server.RespondError(w, http.StatusInternalServerError, err.Error())
			return
		}

		// Check if this is a CLI login (state starts with "cli:")
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

	// Token login — lightweight auth without OAuth.
	r.Post("/api/auth/token-login", func(w http.ResponseWriter, r *http.Request) {
		if a.tokenAuth == nil || !a.tokenAuth.IsConfigured() {
			server.RespondError(w, http.StatusServiceUnavailable, "token auth not configured")
			return
		}

		// Support both JSON and form-encoded
		var token string
		contentType := r.Header.Get("Content-Type")
		if strings.Contains(contentType, "application/json") {
			var req struct {
				Token string `json:"token"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
				token = req.Token
			}
		} else {
			r.ParseForm()
			token = r.FormValue("token")
		}

		if token == "" {
			server.RespondError(w, http.StatusBadRequest, "token is required")
			return
		}

		result, err := a.tokenAuth.HandleLogin(token)
		if err != nil {
			server.RespondError(w, http.StatusUnauthorized, "invalid token")
			return
		}

		// Set session cookie (same as Google OAuth)
		http.SetCookie(w, &http.Cookie{
			Name:     auth.CookieName,
			Value:    result.Session.ID,
			Path:     "/",
			MaxAge:   30 * 24 * 60 * 60,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		})

		// Form submissions: redirect to /
		if !strings.Contains(contentType, "application/json") {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}

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

// LoginPage returns an HTTP handler that shows a login page when the user
// is not authenticated, and calls next when they are. If next is nil,
// a default welcome page is shown for authenticated users.
func (a *App) LoginPage(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if auth.UserID(r) == "" {
			auth.ServeLoginPage(w, r, a.name, a.google, a.tokenAuth)
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

// AccessToken returns the OAuth access token from the request context.
// Returns empty string for local/desktop sessions without OAuth.
// Convenience wrapper around auth.AccessToken.
func AccessToken(r *http.Request) string {
	return auth.AccessToken(r)
}
