package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORS_NoOriginsConfigured(t *testing.T) {
	srv := New()
	srv.Router().Get("/api/test", func(w http.ResponseWriter, r *http.Request) {
		RespondJSON(w, http.StatusOK, map[string]string{"ok": "true"})
	})

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Origin", "http://evil.com")
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("expected no CORS header when no origins configured")
	}
}

func TestCORS_WildcardOrigin(t *testing.T) {
	srv := New(Config{AllowedOrigins: []string{"*"}})
	srv.Router().Get("/api/test", func(w http.ResponseWriter, r *http.Request) {
		RespondJSON(w, http.StatusOK, map[string]string{"ok": "true"})
	})

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Origin", "http://anything.com")
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("got %q, want %q", rec.Header().Get("Access-Control-Allow-Origin"), "*")
	}
}

func TestCORS_SpecificOriginAllowed(t *testing.T) {
	srv := New(Config{AllowedOrigins: []string{"http://app.local:3000"}})
	srv.Router().Get("/api/test", func(w http.ResponseWriter, r *http.Request) {
		RespondJSON(w, http.StatusOK, map[string]string{"ok": "true"})
	})

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Origin", "http://app.local:3000")
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	if rec.Header().Get("Access-Control-Allow-Origin") != "http://app.local:3000" {
		t.Errorf("got %q, want %q", rec.Header().Get("Access-Control-Allow-Origin"), "http://app.local:3000")
	}
	if rec.Header().Get("Vary") != "Origin" {
		t.Error("expected Vary: Origin for specific origin")
	}
}

func TestCORS_SpecificOriginRejected(t *testing.T) {
	srv := New(Config{AllowedOrigins: []string{"http://app.local:3000"}})
	srv.Router().Get("/api/test", func(w http.ResponseWriter, r *http.Request) {
		RespondJSON(w, http.StatusOK, map[string]string{"ok": "true"})
	})

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Origin", "http://evil.com")
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("expected no CORS header for non-allowed origin")
	}
}

func TestCORS_PreflightOptions(t *testing.T) {
	srv := New(Config{AllowedOrigins: []string{"http://app.local:3000"}})
	srv.Router().Get("/api/test", func(w http.ResponseWriter, r *http.Request) {
		RespondJSON(w, http.StatusOK, map[string]string{"ok": "true"})
	})

	req := httptest.NewRequest("OPTIONS", "/api/test", nil)
	req.Header.Set("Origin", "http://app.local:3000")
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("OPTIONS got %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Error("expected Allow-Methods header on preflight")
	}
}

func TestCORS_NonAPIPathNoHeaders(t *testing.T) {
	srv := New(Config{AllowedOrigins: []string{"*"}})
	srv.Router().Get("/page", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("html"))
	})

	req := httptest.NewRequest("GET", "/page", nil)
	req.Header.Set("Origin", "http://anything.com")
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("expected no CORS headers on non-API path")
	}
}

func TestRespondJSON_Success(t *testing.T) {
	rec := httptest.NewRecorder()
	RespondJSON(rec, http.StatusOK, map[string]string{"hello": "world"})

	if rec.Code != http.StatusOK {
		t.Errorf("got %d, want %d", rec.Code, http.StatusOK)
	}

	var body map[string]string
	json.NewDecoder(rec.Body).Decode(&body)
	if body["hello"] != "world" {
		t.Errorf("body = %v", body)
	}
}

func TestRespondJSON_MarshalError(t *testing.T) {
	rec := httptest.NewRecorder()
	// channels can't be marshaled
	RespondJSON(rec, http.StatusOK, make(chan int))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("got %d, want %d for marshal error", rec.Code, http.StatusInternalServerError)
	}
}

func TestRespondError(t *testing.T) {
	rec := httptest.NewRecorder()
	RespondError(rec, http.StatusBadRequest, "bad input")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("got %d, want %d", rec.Code, http.StatusBadRequest)
	}

	var body map[string]string
	json.NewDecoder(rec.Body).Decode(&body)
	if body["error"] != "bad input" {
		t.Errorf("error = %q, want %q", body["error"], "bad input")
	}
}

func TestServerPort(t *testing.T) {
	srv := New(Config{Port: "4567"})
	if srv.Port() != "4567" {
		t.Errorf("port = %q, want %q", srv.Port(), "4567")
	}
}
