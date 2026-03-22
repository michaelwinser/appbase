# Desktop Mode (Wails)

This example can run as a native desktop app using [Wails](https://wails.io).

## How it works

Wails wraps the Go HTTP handler as a native window. The Svelte frontend
makes the same `/api/*` fetch calls — Wails routes them to the Go handler
internally. No ports, no browser.

## Prerequisites

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

## Project structure for desktop mode

Real apps (not examples) should structure their code for dual-mode:

```
myapp/
├── internal/
│   ├── api/          # Shared HTTP handler (generated from OpenAPI)
│   └── store/        # Shared entity store
├── cmd/
│   ├── server/       # Web server main (appbase.Serve)
│   │   └── main.go
│   └── desktop/      # Wails desktop main
│       └── main.go
├── frontend/
│   ├── src/          # Svelte app
│   └── dist/         # Built assets (embedded)
└── wails.json
```

Both `cmd/server/main.go` and `cmd/desktop/main.go` import from `internal/`.
See `../projects/electrician` for a working example of this pattern.

## Desktop main.go template

```go
package main

import (
    "embed"
    "log"

    "github.com/wailsapp/wails/v2"
    "github.com/wailsapp/wails/v2/pkg/options"
    "github.com/wailsapp/wails/v2/pkg/options/assetserver"

    "github.com/myorg/myapp/internal/api"
    "github.com/michaelwinser/appbase"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
    app, _ := appbase.New(appbase.Config{Name: "myapp", Quiet: true})
    defer app.Close()

    // Register routes (same as server mode)
    api.RegisterRoutes(app)

    err := wails.Run(&options.App{
        Title:  "My App",
        Width:  1024,
        Height: 768,
        AssetServer: &assetserver.Options{
            Assets:  assets,
            Handler: app.Handler(),  // Routes /api/* to Go
        },
    })
    if err != nil {
        log.Fatal(err)
    }
}
```

## Key differences from server mode

| Concern | Server mode | Desktop mode |
|---------|------------|--------------|
| Port | Listens on PORT | None (internal) |
| Auth | Google OAuth via browser | Skip auth (single user) |
| Frontend | Embedded in binary, served via HTTP | Embedded, served by Wails asset server |
| Database | Same SQLite file | Same SQLite file |
| User ID | From session cookie | Hardcoded "desktop-user" |

## Database safety

SQLite file locking prevents two processes from writing simultaneously.
Don't run the desktop app and web server against the same database file
at the same time.
