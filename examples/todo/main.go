// Example todo app demonstrating appbase usage.
//
// This serves as both documentation and integration test for the appbase module.
//
// Run:
//
//	go run ./examples/todo
//
// Then:
//
//	curl http://localhost:3000/health
//	curl http://localhost:3000/api/todos
package main

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/michaelwinser/appbase"
	"github.com/michaelwinser/appbase/server"
)

// Schema for the todo app's domain tables.
const schema = `
CREATE TABLE IF NOT EXISTS todos (
	id TEXT PRIMARY KEY,
	user_id TEXT NOT NULL,
	title TEXT NOT NULL,
	done INTEGER NOT NULL DEFAULT 0,
	created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_todos_user ON todos(user_id);
`

func main() {
	app, err := appbase.New(appbase.Config{})
	if err != nil {
		log.Fatal(err)
	}
	defer app.Close()

	// Run app-specific migrations
	if err := app.Migrate(schema); err != nil {
		log.Fatal(err)
	}

	store := &TodoStore{db: app.DB()}

	// Register routes
	r := app.Router()
	r.Get("/api/todos", listTodos(store))
	r.Post("/api/todos", createTodo(store))

	log.Fatal(app.Serve())
}

func listTodos(store *TodoStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := appbase.UserID(r)
		if userID == "" {
			userID = "anonymous" // Allow unauthenticated for demo
		}

		todos, err := store.List(userID)
		if err != nil {
			server.RespondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		server.RespondJSON(w, http.StatusOK, todos)
	}
}

func createTodo(store *TodoStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := appbase.UserID(r)
		if userID == "" {
			userID = "anonymous"
		}

		var req struct {
			Title string `json:"title"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			server.RespondError(w, http.StatusBadRequest, "invalid request")
			return
		}
		if req.Title == "" {
			server.RespondError(w, http.StatusBadRequest, "title is required")
			return
		}

		todo, err := store.Create(userID, req.Title)
		if err != nil {
			server.RespondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		server.RespondJSON(w, http.StatusCreated, todo)
	}
}
