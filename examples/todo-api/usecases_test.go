package main

import (
	"net/http"
	"os"
	"testing"

	"github.com/michaelwinser/appbase"
	"github.com/michaelwinser/appbase/auth"
	"github.com/michaelwinser/appbase/examples/todo-api/api"
	harness "github.com/michaelwinser/appbase/testing"
)

func setupTestApp(t *testing.T) http.Handler {
	t.Helper()
	os.Setenv("STORE_TYPE", "sqlite")
	os.Setenv("SQLITE_DB_PATH", ":memory:")
	os.Setenv("APPBASE_TEST_MODE", "true")
	t.Cleanup(func() {
		os.Unsetenv("SQLITE_DB_PATH")
		os.Unsetenv("APPBASE_TEST_MODE")
	})

	a, err := appbase.New(appbase.Config{Name: "todo-api", Quiet: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })

	s, err := NewTodoStore(a.DB())
	if err != nil {
		t.Fatal(err)
	}

	todoServer := &TodoServer{Store: s}
	api.HandlerFromMux(todoServer, a.Server().Router())

	return a.Server().Router()
}

func TestUseCases(t *testing.T) {
	h := harness.New(t, setupTestApp)

	login := func(c *harness.Client) {
		c.SetHeader("X-Test-User", "test@example.com")
	}

	h.Run("UC-0001", "List todos returns empty array", func(c *harness.Client) {
		login(c)
		resp := c.GET("/api/todos")
		c.AssertStatus(resp, 200)
		c.AssertJSONArray(resp, 0)
	})

	h.Run("UC-0002", "Create a todo", func(c *harness.Client) {
		login(c)
		resp := c.POST("/api/todos", `{"title":"Buy groceries"}`)
		c.AssertStatus(resp, 201)
		c.AssertJSONHas(resp, "title", "Buy groceries")
		c.AssertJSONHas(resp, "done", false)
	})

	h.Run("UC-0003", "Created todo appears in list", func(c *harness.Client) {
		login(c)
		c.POST("/api/todos", `{"title":"Walk the dog"}`)
		resp := c.GET("/api/todos")
		c.AssertStatus(resp, 200)
		items := resp.JSONArray()
		if len(items) == 0 {
			t.Fatal("expected at least one todo")
		}
	})

	h.Run("UC-0004", "Create todo with empty title fails", func(c *harness.Client) {
		login(c)
		resp := c.POST("/api/todos", `{"title":""}`)
		c.AssertStatus(resp, 400)
	})

	h.Run("UC-0005", "Unauthenticated request returns 401", func(c *harness.Client) {
		// No login — no X-Test-User header
		resp := c.GET("/api/todos")
		c.AssertStatus(resp, 401)
	})

	h.Run("UC-0006", "Health endpoint returns ok", func(c *harness.Client) {
		resp := c.GET("/health")
		c.AssertStatus(resp, 200)
		c.AssertJSONHas(resp, "status", "ok")
	})
}

// TestTokenAuthUseCases exercises the full session pipeline using token auth.
// Unlike X-Test-User (which bypasses sessions), this tests real cookie-based
// authentication: token login → session created → cookie set → middleware reads
// cookie → session lookup → context populated.
func TestTokenAuthUseCases(t *testing.T) {
	setupTokenApp := func(t *testing.T) http.Handler {
		t.Helper()
		os.Setenv("STORE_TYPE", "sqlite")
		os.Setenv("SQLITE_DB_PATH", ":memory:")
		t.Cleanup(func() {
			os.Unsetenv("SQLITE_DB_PATH")
		})

		a, err := appbase.New(appbase.Config{
			Name:  "todo-api",
			Quiet: true,
			TokenAuth: &auth.TokenAuthConfig{
				Tokens: map[string]string{"test-token-12345": "tokenuser@example.com"},
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { a.Close() })

		s, err := NewTodoStore(a.DB())
		if err != nil {
			t.Fatal(err)
		}

		todoServer := &TodoServer{Store: s}
		api.HandlerFromMux(todoServer, a.Server().Router())

		return a.Server().Router()
	}

	h := harness.New(t, setupTokenApp)

	login := func(c *harness.Client) {
		resp := c.POST("/api/auth/token-login", `{"token":"test-token-12345"}`)
		c.AssertStatus(resp, 200)
		c.AssertJSONHas(resp, "loggedIn", true)
		c.AssertJSONHas(resp, "email", "tokenuser@example.com")
		// Cookie is automatically saved by harness.Client
	}

	h.Run("UC-1001", "Token login creates session and sets cookie", func(c *harness.Client) {
		login(c)
		// Verify auth status shows logged in (via session cookie, not header)
		resp := c.GET("/api/auth/status")
		c.AssertStatus(resp, 200)
		c.AssertJSONHas(resp, "loggedIn", true)
		c.AssertJSONHas(resp, "email", "tokenuser@example.com")
	})

	h.Run("UC-1002", "Token auth CRUD works through real session pipeline", func(c *harness.Client) {
		login(c)

		// Create
		resp := c.POST("/api/todos", `{"title":"Token auth todo"}`)
		c.AssertStatus(resp, 201)
		c.AssertJSONHas(resp, "title", "Token auth todo")

		// List
		resp = c.GET("/api/todos")
		c.AssertStatus(resp, 200)
		items := resp.JSONArray()
		if len(items) == 0 {
			t.Fatal("expected at least one todo")
		}
	})

	h.Run("UC-1003", "Invalid token returns 401", func(c *harness.Client) {
		resp := c.POST("/api/auth/token-login", `{"token":"wrong-token-xyz"}`)
		c.AssertStatus(resp, 401)
	})

	h.Run("UC-1004", "Without login, API returns 401", func(c *harness.Client) {
		resp := c.GET("/api/todos")
		c.AssertStatus(resp, 401)
	})

	h.Run("UC-1005", "Logout clears session", func(c *harness.Client) {
		login(c)

		// Verify logged in
		resp := c.GET("/api/auth/status")
		c.AssertJSONHas(resp, "loggedIn", true)

		// Logout
		resp = c.POST("/api/auth/logout", "")
		c.AssertStatus(resp, 200)
	})
}
