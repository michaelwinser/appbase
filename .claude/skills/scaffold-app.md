---
name: scaffold-app
description: Create a new application that uses appbase
trigger: When the user wants to create a new app built on the appbase module
---

# Scaffolding a New App on appbase

## What You'll Create

```
myapp/
├── CLAUDE.md              # AI session instructions (devcontainer rules, patterns)
├── app.yaml               # App config (name, port, environments, secrets)
├── app.json               # Deploy script compat (name, gcpProject, region)
├── go.mod                 # Depends on github.com/michaelwinser/appbase
├── openapi.yaml           # API spec (source of truth)
├── main.go                # Server + CLI entry point
├── internal/app/          # Shared code (imported by both server and desktop)
│   ├── store.go           # Entity store (store.Collection)
│   └── server.go          # Implements api.ServerInterface
├── api/                   # Generated from openapi.yaml
│   ├── server.gen.go
│   └── client.gen.go
├── frontend/              # Svelte app (built in devcontainer)
│   ├── package.json
│   ├── src/
│   │   ├── App.svelte
│   │   └── lib/
│   │       ├── api.ts           # Typed fetch wrappers
│   │       └── api-types.ts     # Generated from openapi.yaml
│   └── dist/              # Embedded in Go binary
├── .devcontainer/         # Frontend tooling container
│   └── Dockerfile.frontend
├── cmd/desktop/           # Wails desktop entry point (optional)
├── usecases_test.go       # Use case tests (UC-XXXX)
├── e2e/                   # Shell-based E2E tests
├── deploy/
│   ├── Dockerfile
│   └── docker-compose.yml
├── dev                    # Project command script (sources dev-template.sh)
└── .claude/
    └── settings.local.json
```

## Steps

### 1. Initialize the Module

```bash
mkdir myapp && cd myapp
go mod init github.com/michaelwinser/myapp
go get github.com/michaelwinser/appbase
```

### 2. Create CLAUDE.md

**This is critical.** The per-app CLAUDE.md ensures AI sessions follow the right patterns.

```markdown
# myapp

An appbase application. See `../appbase/CLAUDE.md` for the full framework reference.

## Development Tooling

**Do not install Node.js, npm, pnpm, or frontend build tools on the host.**
All frontend tooling runs inside the project's devcontainer.

### Commands

All development tasks go through `./dev`:

- `./dev codegen` — Generate Go server/client + frontend TypeScript types
- `./dev serve` — Start the web server
- `./dev test` — Run Go tests
- `./dev build` — Build the binary

### Frontend work

Never run `npm`, `npx`, `pnpm`, or `yarn` directly. Use:
- `./dev codegen` — Regenerate frontend types from openapi.yaml
- `./dev frontend install` — Install frontend dependencies
- `./dev frontend build` — Build the frontend
- `./dev frontend <cmd>` — Run any command in the frontend devcontainer

The devcontainer starts automatically when needed and stops after the command completes.

### Devcontainer

The project has a `.devcontainer/Dockerfile.frontend` for frontend tooling.
To add a new tool, add it to that Dockerfile — do not install globally or on the host.

## Architecture

- **Config:** `appbase.Config{LocalMode: appcli.IsLocalMode}` for CLI, `LocalMode: true` for desktop
- **CLI commands:** Use `appcli.ClientForCommand(cmd, "myapp", app.Handler())` for API access
- **DB path:** `cfg.DB.SQLitePath = appcli.LocalDataPath + "/app.db"` in setup()
- **Direct store access:** Use `appcli.LocalUserID()` for user identity, not `"cli-user"`
- **Frontend types:** Generated from openapi.yaml — don't hand-write TypeScript interfaces for API types
- **Desktop:** Use `app.LocalHandler()` with Wails, not `app.Handler()`

See `../appbase/docs/migration-local-mode.md` for the full pattern reference.
```

### 3. Create app.json

```json
{
  "name": "myapp",
  "gcpProject": "",
  "region": "us-central1"
}
```

### 4. Create main.go

```go
package main

import (
    "context"
    "embed"
    "fmt"
    "io/fs"
    "net/http"

    "github.com/spf13/cobra"

    "github.com/michaelwinser/appbase"
    appcli "github.com/michaelwinser/appbase/cli"
    "github.com/michaelwinser/myapp/api"
)

//go:embed frontend/dist/*
var frontendDist embed.FS

var (
    app       *appbase.App
    thingStore *ThingStore
)

func setup() error {
    var err error
    cfg := appbase.Config{
        Name:      "myapp",
        Quiet:     !appcli.IsServeCommand,
        LocalMode: appcli.IsLocalMode,
    }
    if appcli.LocalDataPath != "" {
        cfg.DB.SQLitePath = appcli.LocalDataPath + "/app.db"
    }
    app, err = appbase.New(cfg)
    if err != nil {
        return err
    }
    thingStore, err = NewThingStore(app.DB())
    if err != nil {
        return err
    }

    // Register API routes
    thingServer := &ThingServer{Store: thingStore}
    api.HandlerFromMux(thingServer, app.Server().Router())

    return nil
}

func main() {
    cliApp := appcli.New("myapp", "My application", setup)

    cliApp.SetServeFunc(func() error {
        r := app.Server().Router()
        distFS, _ := fs.Sub(frontendDist, "frontend/dist")
        fileServer := http.FileServer(http.FS(distFS))
        r.Handle("/assets/*", fileServer)
        r.Get("/*", app.LoginPage(func(w http.ResponseWriter, r *http.Request) {
            r.URL.Path = "/"
            fileServer.ServeHTTP(w, r)
        }))
        return app.Serve()
    })

    // CLI commands use the generated HTTP client
    listCmd := &cobra.Command{
        Use:   "list",
        Short: "List things",
        RunE: func(cmd *cobra.Command, args []string) error {
            if err := setup(); err != nil {
                return err
            }
            httpClient, baseURL, cleanup, err := appcli.ClientForCommand(cmd, "myapp", app.Handler())
            if err != nil {
                return err
            }
            defer cleanup()
            client, _ := api.NewClientWithResponses(baseURL, api.WithHTTPClient(httpClient))
            resp, err := client.ListThingsWithResponse(context.Background())
            if err != nil {
                return err
            }
            for _, t := range *resp.JSON200 {
                fmt.Println(t.Name)
            }
            return nil
        },
    }
    cliApp.AddCommand(listCmd)

    cliApp.Execute()
}
```

### 5. Create the store

Use `store.Collection[T]` for automatic SQL/Firestore dual-backend:

```go
// internal/app/store.go
type ThingEntity struct {
    ID        string `json:"id"        store:"id,pk"`
    UserID    string `json:"userId"    store:"user_id,index"`
    Name      string `json:"name"      store:"name"`
    CreatedAt string `json:"createdAt" store:"created_at"`
}
```

See `examples/todo-api/internal/app/store.go` for the complete pattern.

### 6. Create the devcontainer

```dockerfile
# .devcontainer/Dockerfile.frontend
FROM node:20-alpine
ENV PNPM_HOME="/root/.local/share/pnpm"
ENV PATH="$PNPM_HOME:$PATH"
RUN corepack enable && corepack prepare pnpm@latest --activate
RUN pnpm add -g openapi-typescript
RUN apk add --no-cache curl git bash
WORKDIR /app
```

This is the appbase base pattern. Add app-specific tools (e.g., Tailwind, additional codegen) to this file.

### 7. Create the `./dev` script

```sh
#!/bin/sh
set -e
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

# Source shared dev functions from appbase
. "../appbase/deploy/dev-template.sh"

APP_BINARY_NAME="myapp"

case "${1:-help}" in
    *)  dev_dispatch "$@" ;;
esac
```

Make it executable: `chmod +x dev`

### 8. Create the frontend

Initialize in the devcontainer:
```bash
./dev frontend "pnpm create vite frontend --template svelte-ts"
./dev frontend "cd frontend && pnpm install"
```

Add the API client pattern — see `examples/todo-api/frontend/src/lib/api.ts`.
Run `./dev codegen` to generate `api-types.ts` from `openapi.yaml`.

### 9. Configure Claude Code permissions

Create `.claude/settings.local.json`:

```json
{
  "permissions": {
    "allow": [
      "Bash(go:*)", "Bash(git:*)", "Bash(sh:*)",
      "Bash(appbase:*)", "Bash(docker:*)",
      "Bash(gh:*)", "Bash(./dev:*)"
    ]
  }
}
```

### 10. Verify

```bash
go build ./...
go test -v ./...
go run . serve    # Server at localhost:3000
go run . list     # CLI local mode
./dev codegen     # Go + frontend types
```

## Choosing a Pattern

| Pattern | When to use | Example |
|---------|------------|---------|
| **API-first (recommended)** | Apps with a web UI, CLI client, or external consumers | `examples/todo-api/` |
| **Hand-written routes** | Simple apps, internal tools, prototypes | `examples/todo-store/` |
| **Desktop (Wails)** | Native desktop app, same API | `examples/todo-api/DESKTOP.md` |

For API-first apps, see the `api-first` skill for the full workflow.

## Key Patterns

- **Three runtime modes** — local CLI (in-process transport), web server (full OAuth), remote CLI (keychain session)
- **In-process transport** — CLI commands call the handler directly via `ClientForCommand`, no TCP
- **Login page is built-in** — `app.LoginPage(handler)` shows Google sign-in. Skipped in LocalMode.
- **CLI uses the API** — CLI commands use the generated HTTP client via `ClientForCommand`
- **Desktop via LocalHandler()** — use `app.LocalHandler()` with Wails for native desktop apps
- **Frontend types from spec** — `./dev codegen` generates both Go and TypeScript from `openapi.yaml`
- **Devcontainer for frontend** — all Node/pnpm/vite runs in the container, never on the host
