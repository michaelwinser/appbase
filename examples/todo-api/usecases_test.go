package main

import (
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/michaelwinser/appbase"
	"github.com/michaelwinser/appbase/auth"
	"github.com/michaelwinser/appbase/examples/todo-api/api"
	harness "github.com/michaelwinser/appbase/testing"
)

var testSessions *auth.SessionStore

func setupTestApp(t *testing.T) http.Handler {
	t.Helper()
	os.Setenv("STORE_TYPE", "sqlite")
	os.Setenv("SQLITE_DB_PATH", ":memory:")
	t.Cleanup(func() {
		os.Unsetenv("SQLITE_DB_PATH")
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

	// Register generated routes on the chi router
	todoServer := &TodoServer{store: s}
	api.HandlerFromMux(todoServer, a.Server().Router())

	testSessions = a.Sessions()
	return a.Server().Router()
}

func TestUseCases(t *testing.T) {
	h := harness.New(t, setupTestApp)

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

	h.Run("UC-0005", "Unauthenticated request returns 401", func(c *harness.Client) {
		resp := c.GET("/api/todos")
		c.AssertStatus(resp, 401)
	})

	h.Run("UC-0006", "Health endpoint returns ok", func(c *harness.Client) {
		resp := c.GET("/health")
		c.AssertStatus(resp, 200)
		c.AssertJSONHas(resp, "status", "ok")
	})
}
