// Desktop wrapper for the todo-api app using Wails.
//
// Build:
//
//	cd examples/todo-api/cmd/desktop && wails build
//
// Or via ./dev:
//
//	./dev build desktop
package main

import (
	"embed"
	"log"
	"net/http"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"github.com/michaelwinser/appbase"
	appcli "github.com/michaelwinser/appbase/cli"
	"github.com/michaelwinser/appbase/examples/todo-api/api"
	todoapp "github.com/michaelwinser/appbase/examples/todo-api/internal/app"
	"github.com/michaelwinser/appbase/server"
)

//go:embed all:dist
var assets embed.FS

// App struct for Wails binding (minimal)
type App struct{}

func main() {
	appcli.SetupLocalMode("todo-api")

	app, err := appbase.New(appbase.Config{Name: "todo-api", Quiet: true})
	if err != nil {
		log.Fatal(err)
	}
	defer app.Close()

	todoStore, err := todoapp.NewTodoStore(app.DB())
	if err != nil {
		log.Fatal(err)
	}

	todoServer := &todoapp.TodoServer{Store: todoStore}
	api.HandlerFromMux(todoServer, app.Server().Router())

	// Always logged in for desktop mode
	app.Server().Router().Get("/api/auth/status", func(w http.ResponseWriter, r *http.Request) {
		server.RespondJSON(w, http.StatusOK, map[string]interface{}{
			"loggedIn": true,
			"email":    "desktop-user",
		})
	})

	wailsApp := &App{}
	err = wails.Run(&options.App{
		Title:  "Todo",
		Width:  800,
		Height: 600,
		AssetServer: &assetserver.Options{
			Assets:  assets,
			Handler: app.Handler(),
		},
		Bind: []interface{}{wailsApp},
	})
	if err != nil {
		log.Fatal(err)
	}
}
