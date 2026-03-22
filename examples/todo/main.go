// Example todo app demonstrating appbase usage.
//
// This serves as both documentation and integration test for the appbase module.
//
// Run as server:
//
//	go run ./examples/todo serve
//
// Run CLI commands:
//
//	go run ./examples/todo add "Buy groceries"
//	go run ./examples/todo list
//	go run ./examples/todo version
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/michaelwinser/appbase"
	appcli "github.com/michaelwinser/appbase/cli"
	"github.com/michaelwinser/appbase/server"
)

var (
	app   *appbase.App
	todos *TodoStore
)

func setup() error {
	var err error
	app, err = appbase.New(appbase.Config{Name: "Todo"})
	if err != nil {
		return err
	}
	todos, err = NewTodoStore(app.DB())
	if err != nil {
		return err
	}
	return nil
}

func main() {
	cli := appcli.New("todo", "A simple todo app built on appbase", setup)

	// Override serve to register routes and start
	cli.SetServeFunc(func() error {
		r := app.Router()
		r.Get("/api/todos", listHandler)
		r.Post("/api/todos", createHandler)

		// Root page: login when unauthenticated, todo UI when authenticated
		r.Get("/", app.LoginPage(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<!DOCTYPE html>
<html><head><title>Todo - appbase example</title></head>
<body style="font-family:system-ui;max-width:600px;margin:2rem auto;padding:0 1rem">
<h1>Todo</h1>
<p>Signed in as ` + appbase.Email(r) + `. <a href="/health">/health</a> | <a href="/api/todos">/api/todos</a></p>
<form method="POST" action="/api/auth/logout" style="margin-bottom:1rem"><button>Sign out</button></form>
<form onsubmit="event.preventDefault();fetch('/api/todos',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({title:this.t.value})}).then(r=>r.json()).then(()=>{this.t.value='';location.reload()})">
<input name="t" placeholder="Add a todo..." style="padding:8px;width:70%">
<button style="padding:8px 16px">Add</button>
</form>
<ul id="list"></ul>
<script>fetch('/api/todos').then(r=>r.json()).then(todos=>{document.getElementById('list').innerHTML=todos.map(t=>'<li>'+t.title+'</li>').join('')}).catch(()=>{})</script>
</body></html>`))
		}))

		return app.Serve()
	})

	// CLI: add a todo
	addCmd := cli.Command("add", "Add a new todo", func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("title is required: todo add \"Buy groceries\"")
		}
		todo, err := todos.Create("cli-user", args[0])
		if err != nil {
			return err
		}
		fmt.Printf("Created: %s (id: %s)\n", todo.Title, todo.ID)
		return nil
	})
	cli.AddCommand(addCmd)

	// CLI: list todos
	listCmd := cli.Command("list", "List all todos", func(cmd *cobra.Command, args []string) error {
		items, err := todos.List("cli-user")
		if err != nil {
			return err
		}
		if len(items) == 0 {
			fmt.Println("No todos yet. Add one with: todo add \"Buy groceries\"")
			return nil
		}
		for _, t := range items {
			status := "[ ]"
			if t.Done {
				status = "[x]"
			}
			fmt.Printf("%s %s  (%s)\n", status, t.Title, t.ID[:8])
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
	items, err := todos.List(userID)
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
		Title string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Title == "" {
		server.RespondError(w, http.StatusBadRequest, "title is required")
		return
	}
	todo, err := todos.Create(userID, req.Title)
	if err != nil {
		server.RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	server.RespondJSON(w, http.StatusCreated, todo)
}

func init() {
	// Ensure data directory exists for SQLite
	if err := ensureDataDir(); err != nil {
		log.Printf("Warning: could not create data directory: %v", err)
	}
}

func ensureDataDir() error {
	return nil // SQLite creates the file; directory must exist
}
