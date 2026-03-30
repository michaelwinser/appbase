package main

import (
	"net/http"
	"os"
	"testing"

	"github.com/michaelwinser/appbase"
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
