# Desktop Mode (Wails)

This example runs as a native desktop app using [Wails](https://wails.io).

## Build

```bash
./dev build desktop     # builds macOS .app bundle
./dev build             # builds server + desktop
```

Requires: `go install github.com/wailsapp/wails/v2/cmd/wails@latest`

## How it works

Wails wraps the Go HTTP handler as a native window. The Svelte frontend
makes the same `/api/*` fetch calls — Wails routes them to the Go handler
internally. No ports, no browser.

## Project structure

Shared code lives in `internal/app/` so both entry points can import it:

```
myapp/
├── internal/app/         # Shared store + server implementation
│   ├── store.go          # Entity store (store.Collection)
│   └── server.go         # Implements api.ServerInterface
├── main.go               # Server + CLI entry point
├── store.go              # Re-exports from internal/app
├── cmd/desktop/          # Wails desktop entry point
│   ├── main.go           # Uses internal/app + appcli.SetupLocalMode
│   ├── wails.json        # Wails project config
│   ├── dist/             # Frontend assets (copied from frontend/dist)
│   ├── frontend/         # Empty dir (Wails requirement)
│   └── build/
│       └── darwin/
│           └── Info.plist
├── frontend/
│   ├── src/              # Svelte app
│   └── dist/             # Built assets
├── api/                  # Generated from OpenAPI
└── openapi.yaml
```

## Desktop main.go pattern

```go
package main

import (
    "embed"
    "log"

    "github.com/wailsapp/wails/v2"
    "github.com/wailsapp/wails/v2/pkg/options"
    "github.com/wailsapp/wails/v2/pkg/options/assetserver"

    "github.com/michaelwinser/appbase"
    appcli "github.com/michaelwinser/appbase/cli"
    "github.com/myorg/myapp/api"
    todoapp "github.com/myorg/myapp/internal/app"
)

//go:embed all:dist
var assets embed.FS

type App struct{}

func main() {
    // Set up local mode: ~/.config/myapp/app.db, no auth
    appcli.SetupLocalMode("myapp")

    app, _ := appbase.New(appbase.Config{Name: "myapp", Quiet: true})
    defer app.Close()

    store, _ := todoapp.NewTodoStore(app.DB())
    server := &todoapp.TodoServer{Store: store}
    api.HandlerFromMux(server, app.Server().Router())

    // Auth status — always logged in for desktop
    app.Server().Router().Get("/api/auth/status", func(w, r) {
        server.RespondJSON(w, 200, map[string]interface{}{"loggedIn": true, "email": "desktop-user"})
    })

    wails.Run(&options.App{
        Title: "My App", Width: 1024, Height: 768,
        AssetServer: &assetserver.Options{
            Assets: assets, Handler: app.Handler(),
        },
        Bind: []interface{}{&App{}},
    })
}
```

Key elements:
- `appcli.SetupLocalMode("myapp")` — creates `~/.config/myapp/`, sets `AUTH_MODE=dev`
- `internal/app` — shared types, not in the `main` package
- `//go:embed all:dist` — frontend assets embedded from `dist/` (not `frontend/dist`)
- `app.Handler()` — routes `/api/*` to Go, Wails serves static assets

## Key differences from server mode

| Concern | Server mode | Desktop mode |
|---------|------------|--------------|
| Port | Listens on PORT | None (internal) |
| Auth | Google OAuth via browser | Auto (single user) |
| Data | `data/app.db` or app.yaml config | `~/.config/<appname>/app.db` |
| Frontend | Embedded, served via HTTP | Embedded, served by Wails |
| Build | `go build .` | `wails build` (via `./dev build desktop`) |

## Database safety

SQLite file locking prevents two processes from writing simultaneously.
Don't run the desktop app and web server against the same database file.
