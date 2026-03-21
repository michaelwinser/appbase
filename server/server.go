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

// New creates a new server with standard middleware (logger, recoverer, CORS, JSON content-type).
func New() *Server {
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(apiMiddleware)

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

// apiMiddleware applies JSON content-type and CORS to API routes.
func apiMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/health" {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// RespondJSON writes a JSON response.
func RespondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// RespondError writes a JSON error response.
func RespondError(w http.ResponseWriter, status int, message string) {
	RespondJSON(w, status, map[string]string{"error": message})
}
