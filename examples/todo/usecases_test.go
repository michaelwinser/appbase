package main

import (
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/michaelwinser/appbase"
	"github.com/michaelwinser/appbase/auth"
	harness "github.com/michaelwinser/appbase/testing"
)

var testSessions *auth.SessionStore

// Use cases from the todo app PRD.
// Each test is named UC-XXXX and maps to a documented use case.

func setupTestApp(t *testing.T) http.Handler {
	t.Helper()
	os.Setenv("STORE_TYPE", "sqlite")
	os.Setenv("SQLITE_DB_PATH", ":memory:")
	t.Cleanup(func() {
		os.Unsetenv("SQLITE_DB_PATH")
	})

	a, err := appbase.New(appbase.Config{Name: "Todo", Quiet: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })

	if err := a.Migrate(schema); err != nil {
		t.Fatal(err)
	}

	s := NewTodoStore(a.DB())

	r := a.Server().Router()
	r.Get("/api/todos", func(w http.ResponseWriter, r *http.Request) {
		listHandler(w, r)
	})
	r.Post("/api/todos", func(w http.ResponseWriter, r *http.Request) {
		createHandler(w, r)
	})
	r.Get("/", a.LoginPage(nil))

	// Need to set the package-level store for handlers
	store = s
	testSessions = a.Sessions()

	return r
}

func TestUseCases(t *testing.T) {
	h := harness.New(t, setupTestApp)

	// Helper to authenticate a test client
	login := func(c *harness.Client) {
		session, err := testSessions.Create("test@example.com", "test@example.com", 1*time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		c.SetCookie(auth.CookieName, session.ID)
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

	h.Run("UC-0005", "Create todo with invalid JSON fails", func(c *harness.Client) {
		login(c)
		resp := c.POST("/api/todos", `not json`)
		c.AssertStatus(resp, 400)
	})

	h.Run("UC-0007", "Unauthenticated request returns 401", func(c *harness.Client) {
		resp := c.GET("/api/todos")
		c.AssertStatus(resp, 401)
	})

	h.Run("UC-0006", "Health endpoint returns ok", func(c *harness.Client) {
		resp := c.GET("/health")
		c.AssertStatus(resp, 200)
		c.AssertJSONHas(resp, "status", "ok")
	})

	h.Run("UC-0008", "Root page shows login when unauthenticated", func(c *harness.Client) {
		resp := c.GET("/")
		c.AssertStatus(resp, 200)
		body := string(resp.Body)
		if !strings.Contains(body, "Todo") || !strings.Contains(body, "<!DOCTYPE html>") {
			t.Fatal("expected login page HTML")
		}
	})

	h.Run("UC-0009", "Root page shows content when authenticated", func(c *harness.Client) {
		login(c)
		resp := c.GET("/")
		c.AssertStatus(resp, 200)
		body := string(resp.Body)
		if !strings.Contains(body, "Signed in as") {
			t.Fatal("expected authenticated content with 'Signed in as'")
		}
	})
}
