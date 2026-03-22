package main

import (
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/michaelwinser/appbase"
	"github.com/michaelwinser/appbase/auth"
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

	a, err := appbase.New(appbase.Config{Name: "Bookmarks"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })

	s, err := NewBookmarkStore(a.DB())
	if err != nil {
		t.Fatal(err)
	}

	r := a.Server().Router()
	r.Get("/api/bookmarks", func(w http.ResponseWriter, r *http.Request) {
		listHandler(w, r)
	})
	r.Post("/api/bookmarks", func(w http.ResponseWriter, r *http.Request) {
		createHandler(w, r)
	})

	bookmarks = s
	testSessions = a.Sessions()

	return r
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

	h.Run("UC-0001", "List bookmarks returns empty array", func(c *harness.Client) {
		login(c)
		resp := c.GET("/api/bookmarks")
		c.AssertStatus(resp, 200)
		c.AssertJSONArray(resp, 0)
	})

	h.Run("UC-0002", "Create a bookmark", func(c *harness.Client) {
		login(c)
		resp := c.POST("/api/bookmarks", `{"url":"https://example.com","title":"Example","tags":"test"}`)
		c.AssertStatus(resp, 201)
		c.AssertJSONHas(resp, "url", "https://example.com")
		c.AssertJSONHas(resp, "title", "Example")
		c.AssertJSONHas(resp, "tags", "test")
	})

	h.Run("UC-0003", "Created bookmark appears in list", func(c *harness.Client) {
		login(c)
		c.POST("/api/bookmarks", `{"url":"https://go.dev","title":"Go"}`)
		resp := c.GET("/api/bookmarks")
		c.AssertStatus(resp, 200)
		items := resp.JSONArray()
		if len(items) == 0 {
			t.Fatal("expected at least one bookmark")
		}
	})

	h.Run("UC-0004", "Create bookmark with empty URL fails", func(c *harness.Client) {
		login(c)
		resp := c.POST("/api/bookmarks", `{"url":"","title":"No URL"}`)
		c.AssertStatus(resp, 400)
	})

	h.Run("UC-0005", "Unauthenticated request returns 401", func(c *harness.Client) {
		resp := c.GET("/api/bookmarks")
		c.AssertStatus(resp, 401)
	})

	h.Run("UC-0006", "Title defaults to URL when omitted", func(c *harness.Client) {
		login(c)
		resp := c.POST("/api/bookmarks", `{"url":"https://bare.example.com"}`)
		c.AssertStatus(resp, 201)
		c.AssertJSONHas(resp, "title", "https://bare.example.com")
	})
}
