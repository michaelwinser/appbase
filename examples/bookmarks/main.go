// Example bookmarks app using the store.Collection abstraction.
//
// Demonstrates a slightly richer entity than the todo example,
// with URL, title, tags, and delete support.
//
// Run as server:
//
//	go run ./examples/bookmarks serve
//
// Run CLI commands:
//
//	go run ./examples/bookmarks add "https://example.com" "Example Site"
//	go run ./examples/bookmarks list
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/spf13/cobra"

	"github.com/michaelwinser/appbase"
	appcli "github.com/michaelwinser/appbase/cli"
	"github.com/michaelwinser/appbase/server"
)

var (
	app       *appbase.App
	bookmarks *BookmarkStore
)

func setup() error {
	var err error
	app, err = appbase.New(appbase.Config{Name: "Bookmarks"})
	if err != nil {
		return err
	}
	bookmarks, err = NewBookmarkStore(app.DB())
	if err != nil {
		return err
	}
	return nil
}

func main() {
	cli := appcli.New("bookmarks", "A bookmark manager built on appbase", setup)

	cli.SetServeFunc(func() error {
		r := app.Router()
		r.Get("/api/bookmarks", listHandler)
		r.Post("/api/bookmarks", createHandler)
		r.Delete("/api/bookmarks/{id}", deleteHandler)

		r.Get("/", app.LoginPage(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<!DOCTYPE html>
<html><head><title>Bookmarks</title></head>
<body style="font-family:system-ui;max-width:600px;margin:2rem auto;padding:0 1rem">
<h1>Bookmarks</h1>
<p>Signed in as ` + appbase.Email(r) + `.</p>
<form method="POST" action="/api/auth/logout" style="margin-bottom:1rem"><button>Sign out</button></form>
<form onsubmit="event.preventDefault();fetch('/api/bookmarks',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({url:this.u.value,title:this.t.value,tags:this.g.value})}).then(r=>r.json()).then(()=>{this.u.value='';this.t.value='';this.g.value='';location.reload()})">
<input name="u" placeholder="URL" style="padding:8px;width:90%;margin-bottom:4px"><br>
<input name="t" placeholder="Title" style="padding:8px;width:60%">
<input name="g" placeholder="Tags" style="padding:8px;width:25%">
<button style="padding:8px 16px">Save</button>
</form>
<ul id="list"></ul>
<script>fetch('/api/bookmarks').then(r=>r.json()).then(items=>{document.getElementById('list').innerHTML=items.map(b=>'<li><a href="'+b.url+'">'+b.title+'</a> <small>'+b.tags+'</small></li>').join('')}).catch(()=>{})</script>
</body></html>`))
		}))

		return app.Serve()
	})

	addCmd := cli.Command("add", "Add a bookmark", func(cmd *cobra.Command, args []string) error {
		if len(args) < 2 {
			return fmt.Errorf("usage: bookmarks add <url> <title> [tags]")
		}
		tags := ""
		if len(args) > 2 {
			tags = args[2]
		}
		b, err := bookmarks.Create("cli-user", args[0], args[1], tags)
		if err != nil {
			return err
		}
		fmt.Printf("Saved: %s — %s (id: %s)\n", b.Title, b.URL, b.ID[:8])
		return nil
	})
	cli.AddCommand(addCmd)

	listCmd := cli.Command("list", "List bookmarks", func(cmd *cobra.Command, args []string) error {
		items, err := bookmarks.List("cli-user")
		if err != nil {
			return err
		}
		if len(items) == 0 {
			fmt.Println("No bookmarks yet.")
			return nil
		}
		for _, b := range items {
			tags := ""
			if b.Tags != "" {
				tags = " [" + b.Tags + "]"
			}
			fmt.Printf("  %s — %s%s\n", b.Title, b.URL, tags)
		}
		return nil
	})
	cli.AddCommand(listCmd)

	cli.Execute()
}

func listHandler(w http.ResponseWriter, r *http.Request) {
	userID := appbase.UserID(r)
	if userID == "" {
		userID = "anonymous"
	}
	items, err := bookmarks.List(userID)
	if err != nil {
		server.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	server.RespondJSON(w, http.StatusOK, items)
}

func createHandler(w http.ResponseWriter, r *http.Request) {
	userID := appbase.UserID(r)
	if userID == "" {
		userID = "anonymous"
	}
	var req struct {
		URL   string `json:"url"`
		Title string `json:"title"`
		Tags  string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.URL == "" {
		server.RespondError(w, http.StatusBadRequest, "url is required")
		return
	}
	if req.Title == "" {
		req.Title = req.URL
	}
	b, err := bookmarks.Create(userID, req.URL, req.Title, req.Tags)
	if err != nil {
		server.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	server.RespondJSON(w, http.StatusCreated, b)
}

func deleteHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := bookmarks.Delete(id); err != nil {
		server.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	server.RespondJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func init() {
	if err := ensureDataDir(); err != nil {
		log.Printf("Warning: could not create data directory: %v", err)
	}
}

func ensureDataDir() error {
	return nil
}
