// Package server provides HTTP server scaffolding for appbase applications.
package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"


	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// Server wraps a chi router with common middleware and configuration.
type Server struct {
	router chi.Router
	port   string
}

// Config configures the HTTP server.
type Config struct {
	// AllowedOrigins for CORS. If empty, CORS headers are not set (same-origin only).
	// Set to ["*"] to allow all origins (public APIs only — not recommended with auth).
	AllowedOrigins []string
}

// New creates a new server with standard middleware (logger, recoverer, CORS, JSON content-type).
func New(configs ...Config) *Server {
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	var cfg Config
	if len(configs) > 0 {
		cfg = configs[0]
	}

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(corsMiddleware(cfg.AllowedOrigins))

	return &Server{router: r, port: port}
}

// Router returns the chi router for registering routes.
func (s *Server) Router() chi.Router {
	return s.router
}

// Serve starts the HTTP server. Blocks until the server exits.
func (s *Server) Serve() error {
	addr := fmt.Sprintf(":%s", s.port)
	log.Printf("Server starting on %s", addr)
	return http.ListenAndServe(addr, s.router)
}

// Port returns the configured port.
func (s *Server) Port() string {
	return s.port
}

// corsMiddleware applies JSON content-type and CORS to API routes.
// If allowedOrigins is empty, no CORS headers are set (same-origin only).
func corsMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	originSet := make(map[string]bool, len(allowedOrigins))
	allowAll := false
	for _, o := range allowedOrigins {
		if o == "*" {
			allowAll = true
		}
		originSet[o] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/health" {
				w.Header().Set("Content-Type", "application/json")

				if len(allowedOrigins) > 0 {
					origin := r.Header.Get("Origin")
					if allowAll {
						w.Header().Set("Access-Control-Allow-Origin", "*")
					} else if originSet[origin] {
						w.Header().Set("Access-Control-Allow-Origin", origin)
						w.Header().Set("Vary", "Origin")
					}
					w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
					w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
					if r.Method == "OPTIONS" {
						w.WriteHeader(http.StatusOK)
						return
					}
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RespondJSON writes a JSON response.
// Encodes to a buffer first so encoding errors are caught before writing headers.
func RespondJSON(w http.ResponseWriter, status int, data interface{}) {
	buf, err := json.Marshal(data)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"failed to encode response"}`))
		log.Printf("RespondJSON: marshal error: %v", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(buf)
	w.Write([]byte("\n"))
}

// RespondError writes a JSON error response.
func RespondError(w http.ResponseWriter, status int, message string) {
	RespondJSON(w, status, map[string]string{"error": message})
}
