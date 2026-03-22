// Example todo app demonstrating API-first development with OpenAPI codegen.
//
// Compare with:
//   - examples/todo/       — raw db connections, hand-written routes
//   - examples/todo-store/ — store.Collection, hand-written routes
//   - examples/todo-api/   — OpenAPI spec, generated server + client, CLI auth
//
// The OpenAPI spec (openapi.yaml) is the source of truth. Server interface
// and client are generated with oapi-codegen.
//
// Server:
//
//	go run ./examples/todo-api serve
//
// CLI (talks to server via HTTP):
//
//	go run ./examples/todo-api login --server http://localhost:3000
//	go run ./examples/todo-api list
//	go run ./examples/todo-api add "Buy groceries"
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/michaelwinser/appbase"
	appcli "github.com/michaelwinser/appbase/cli"
	"github.com/michaelwinser/appbase/examples/todo-api/api"
)

var (
	app       *appbase.App
	todoStore *TodoStore
)

func setup() error {
	var err error
	app, err = appbase.New(appbase.Config{Name: "todo-api"})
	if err != nil {
		return err
	}
	todoStore, err = NewTodoStore(app.DB())
	if err != nil {
		return err
	}
	return nil
}

func main() {
	cliApp := appcli.New("todo-api", "API-first todo app built on appbase", setup)

	cliApp.SetServeFunc(func() error {
		r := app.Router()

		// Register generated routes — the OpenAPI spec defines them
		todoServer := &TodoServer{store: todoStore}
		api.HandlerFromMux(todoServer, app.Server().Router())

		// Root page with login
		r.Get("/", app.LoginPage(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<!DOCTYPE html>
<html><head><title>Todo API</title></head>
<body style="font-family:system-ui;max-width:600px;margin:2rem auto;padding:0 1rem">
<h1>Todo API</h1>
<p>Signed in as ` + appbase.Email(r) + `.</p>
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

	// CLI commands — use the generated HTTP client, not direct store access.
	// The CLI authenticates via browser OAuth (login command) and sends
	// requests to the server.

	addCmd := &cobra.Command{
		Use:   "add [title]",
		Short: "Add a new todo (via API)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			serverFlag, _ := cmd.Flags().GetString("server")
			serverURL := appcli.ResolveServerURL(serverFlag, "todo-api")

			httpClient, err := appcli.AuthenticatedClient("todo-api")
			if err != nil {
				return fmt.Errorf("not logged in — run: todo-api login --server %s", serverURL)
			}

			client, err := api.NewClientWithResponses(serverURL, api.WithHTTPClient(httpClient))
			if err != nil {
				return err
			}

			title := args[0]
			resp, err := client.CreateTodoWithResponse(context.Background(), api.CreateTodoRequest{
				Title: title,
			})
			if err != nil {
				return err
			}
			if resp.StatusCode() != http.StatusCreated {
				return fmt.Errorf("server returned %d: %s", resp.StatusCode(), string(resp.Body))
			}
			fmt.Printf("Created: %s (id: %s)\n", resp.JSON201.Title, resp.JSON201.Id)
			return nil
		},
	}
	cliApp.AddCommand(addCmd)

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List todos (via API)",
		RunE: func(cmd *cobra.Command, args []string) error {
			serverFlag, _ := cmd.Flags().GetString("server")
			serverURL := appcli.ResolveServerURL(serverFlag, "todo-api")

			httpClient, err := appcli.AuthenticatedClient("todo-api")
			if err != nil {
				return fmt.Errorf("not logged in — run: todo-api login --server %s", serverURL)
			}

			client, err := api.NewClientWithResponses(serverURL, api.WithHTTPClient(httpClient))
			if err != nil {
				return err
			}

			resp, err := client.ListTodosWithResponse(context.Background())
			if err != nil {
				return err
			}
			if resp.StatusCode() != http.StatusOK {
				return fmt.Errorf("server returned %d: %s", resp.StatusCode(), string(resp.Body))
			}

			todos := *resp.JSON200
			if len(todos) == 0 {
				fmt.Println("No todos yet. Add one with: todo-api add \"Buy groceries\"")
				return nil
			}
			for _, t := range todos {
				status := "[ ]"
				if t.Done {
					status = "[x]"
				}
				fmt.Printf("%s %s  (%s)\n", status, t.Title, t.Id[:8])
			}
			return nil
		},
	}
	cliApp.AddCommand(listCmd)

	cliApp.Execute()
}

func init() {
	if err := ensureDataDir(); err != nil {
		log.Printf("Warning: could not create data directory: %v", err)
	}
}

func ensureDataDir() error {
	return nil
}
