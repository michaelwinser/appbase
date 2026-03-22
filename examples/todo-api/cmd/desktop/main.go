//go:build desktop

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

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"github.com/michaelwinser/appbase"
	"github.com/michaelwinser/appbase/examples/todo-api/api"
	todoapp "github.com/michaelwinser/appbase/examples/todo-api/internal/app"
)

//go:embed all:dist
var assets embed.FS

// App struct for Wails binding (minimal)
type App struct{}

func main() {
	app, err := appbase.New(appbase.Config{Name: "todo-api", Quiet: true, LocalMode: true})
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

	wailsApp := &App{}
	err = wails.Run(&options.App{
		Title:  "Todo",
		Width:  800,
		Height: 600,
		AssetServer: &assetserver.Options{
			Assets:  assets,
			Handler: app.LocalHandler(),
		},
		Bind: []interface{}{wailsApp},
	})
	if err != nil {
		log.Fatal(err)
	}
}
