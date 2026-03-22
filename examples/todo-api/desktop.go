//go:build desktop

// Desktop mode — builds the app as a Wails native window.
//
// Build:
//
//	go build -tags desktop -o TodoApp .
//
// The same store, server, and frontend are used — Wails just wraps
// them as a native window instead of an HTTP server.
package main

import (
	"embed"
	"log"
	"net/http"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"github.com/michaelwinser/appbase"
	"github.com/michaelwinser/appbase/examples/todo-api/api"
	"github.com/michaelwinser/appbase/server"
	"github.com/michaelwinser/appbase/store"
)

//go:embed frontend/dist/*
var desktopAssets embed.FS

func main() {
	// Initialize app in quiet, local mode
	app, err := appbase.New(appbase.Config{Name: "todo-api", Quiet: true})
	if err != nil {
		log.Fatal(err)
	}
	defer app.Close()

	// Create store — uses ~/.config/todo-api/app.db by default
	coll, err := store.NewCollection[TodoEntity](app.DB(), "todos")
	if err != nil {
		log.Fatal(err)
	}
	todoStore := &TodoStore{coll: coll}

	// Register generated API routes
	todoServer := &TodoServer{store: todoStore}
	api.HandlerFromMux(todoServer, app.Server().Router())

	// Auth status — always logged in for desktop mode
	app.Server().Router().Get("/api/auth/status", func(w http.ResponseWriter, r *http.Request) {
		server.RespondJSON(w, http.StatusOK, map[string]interface{}{
			"loggedIn": true,
			"email":    "desktop-user",
		})
	})

	// Launch Wails window
	err = wails.Run(&options.App{
		Title:  "Todo",
		Width:  800,
		Height: 600,
		AssetServer: &assetserver.Options{
			Assets:  desktopAssets,
			Handler: app.Handler(),
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}
