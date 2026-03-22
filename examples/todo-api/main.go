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
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/michaelwinser/appbase"
	appcli "github.com/michaelwinser/appbase/cli"
	"github.com/michaelwinser/appbase/examples/todo-api/api"
)

//go:embed frontend/dist/*
var frontendDist embed.FS

var (
	app       *appbase.App
	todoStore *TodoStore
)

func setup() error {
	var err error
	app, err = appbase.New(appbase.Config{Name: "todo-api", Quiet: !appcli.IsServeCommand})
	if err != nil {
		return err
	}
	todoStore, err = NewTodoStore(app.DB())
	if err != nil {
		return err
	}

	// Register routes for auto-serve (CLI commands without --server)
	todoServer := &TodoServer{store: todoStore}
	api.HandlerFromMux(todoServer, app.Server().Router())
	appcli.AutoServeHandler = app.Server().Router()

	return nil
}

func main() {
	cliApp := appcli.New("todo-api", "API-first todo app built on appbase", setup)

	cliApp.SetServeFunc(func() error {
		r := app.Server().Router()

		// Serve the Svelte SPA for authenticated users, login page otherwise.
		// The SPA handles routing client-side — all non-API paths serve index.html.
		distFS, err := fs.Sub(frontendDist, "frontend/dist")
		if err != nil {
			return fmt.Errorf("embedding frontend: %w", err)
		}
		fileServer := http.FileServer(http.FS(distFS))

		// Serve static assets (JS, CSS) directly
		r.Handle("/assets/*", fileServer)

		// Root: login page if unauthenticated, SPA if authenticated
		r.Get("/*", app.LoginPage(func(w http.ResponseWriter, r *http.Request) {
			// Serve index.html for all routes (SPA)
			r.URL.Path = "/"
			fileServer.ServeHTTP(w, r)
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
			serverURL, cleanup, err := appcli.ResolveServerWithAutoServe(cmd, "todo-api")
			if err != nil {
				return err
			}
			defer cleanup()

			httpClient, err := appcli.AuthenticatedClient("todo-api")
			if err != nil {
				// Auto-serve mode: use a plain client (no auth needed for local)
				httpClient = http.DefaultClient
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
			serverURL, cleanup, err := appcli.ResolveServerWithAutoServe(cmd, "todo-api")
			if err != nil {
				return err
			}
			defer cleanup()

			httpClient, err := appcli.AuthenticatedClient("todo-api")
			if err != nil {
				httpClient = http.DefaultClient
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
