---
name: scaffold-app
description: Create a new application that uses appbase
trigger: When the user wants to create a new app built on the appbase module
---

# Scaffolding a New App on appbase

## What You'll Create

```
myapp/
├── app.yaml               # App config (name, port, environments, secrets)
├── app.json               # Deploy script compat (name, gcpProject, region)
├── CLAUDE.md              # App-specific AI instructions
├── go.mod                 # Depends on github.com/michaelwinser/appbase
├── openapi.yaml           # API spec (source of truth)
├── main.go                # Server + CLI entry point
├── store.go               # Re-exports from internal/app
├── internal/app/          # Shared code (imported by both server and desktop)
│   ├── store.go           # Entity store (store.Collection)
│   └── server.go          # Implements api.ServerInterface
├── api/                   # Generated from openapi.yaml
│   ├── server.gen.go
│   └── client.gen.go
├── frontend/              # Svelte app (built in devcontainer)
│   ├── src/
│   └── dist/              # Embedded in Go binary
├── cmd/desktop/           # Wails desktop entry point (optional)
│   ├── main.go
│   ├── wails.json
│   └── dist/              # Frontend assets (copied from frontend/dist)
├── usecases_test.go       # Use case tests (UC-XXXX)
├── e2e/                   # Shell-based E2E tests
├── deploy/
│   ├── Dockerfile
│   └── docker-compose.yml
├── dev                    # Project command script (sources dev-template.sh)
└── .github/
    └── workflows/ci.yml   # CI pipeline
```

## Steps

### 1. Initialize the Module

```bash
mkdir myapp && cd myapp
go mod init github.com/michaelwinser/myapp
go get github.com/michaelwinser/appbase
```

### 2. Create app.json

```json
{
  "name": "myapp",
  "gcpProject": "",
  "region": "us-central1"
}
```

The `gcpProject` field is empty until you run provisioning. The `name` is used as the Cloud Run service name and in the login page title.

### 3. Create main.go

```go
package main

import (
    "log"
    "github.com/michaelwinser/appbase"
    appcli "github.com/michaelwinser/appbase/cli"
)

const schema = `
CREATE TABLE IF NOT EXISTS things (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    name TEXT NOT NULL,
    created_at TEXT NOT NULL
);
`

var (
    app   *appbase.App
    store *ThingStore
)

func setup() error {
    var err error
    app, err = appbase.New(appbase.Config{Name: "MyApp"})
    if err != nil {
        return err
    }
    if err := app.Migrate(schema); err != nil {
        return err
    }
    store = NewThingStore(app.DB())
    return nil
}

func main() {
    cli := appcli.New("myapp", "My application", setup)

    cli.SetServeFunc(func() error {
        r := app.Router()

        // Root page: login when unauthenticated, app content when authenticated
        r.Get("/", app.LoginPage(myContentHandler))

        // API routes (require auth via middleware)
        r.Get("/api/things", listHandler)
        r.Post("/api/things", createHandler)

        return app.Serve()
    })

    // Add CLI commands
    cli.AddCommand(cli.Command("list", "List things", listCmd))

    cli.Execute()
}
```

### 4. Create the store (dual-backend)

Define a backend interface and two implementations (SQL + Firestore):

```go
// store.go — interface and factory
type thingBackend interface {
    List(userID string) ([]Thing, error)
    Create(thing *Thing) error
}

func NewThingStore(d *db.DB) *ThingStore {
    if d.IsSQL() {
        return &ThingStore{backend: &sqlThingBackend{db: d}}
    }
    return &ThingStore{backend: &firestoreThingBackend{db: d}}
}

// store_sql.go — SQL queries (SQLite/Postgres)
// store_firestore.go — Firestore document operations
```

Always include `user_id` for multi-tenant queries. See `examples/todo/store*.go` for the complete pattern.

### 5. Create handler.go

HTTP handlers use `appbase.UserID(r)` for auth and `server.RespondJSON`/`server.RespondError` for responses.

### 6. Write the PRD

Create `docs/prd.md` with numbered use cases (UC-XXXX). Each use case has acceptance criteria that map directly to tests.

### 7. Write Use Case Tests

```go
func TestUseCases(t *testing.T) {
    h := harness.New(t, setupTestApp)

    h.Run("UC-0001", "Description from PRD", func(c *harness.Client) {
        login(c)
        resp := c.POST("/api/things", `{"name":"test"}`)
        c.AssertStatus(resp, 201)
    })
}
```

### 8. Create the `./dev` Project Script

Every appbase project uses `./dev` as the standard project command script.
The name is consistent across all projects — nothing to remember.

**Option A: Source the shared template (recommended)**

Sources `deploy/dev-template.sh` from appbase so bug fixes propagate automatically:

```sh
#!/bin/sh
set -e
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

# Source shared dev functions from appbase
. "../appbase/deploy/dev-template.sh"

# App-specific settings
APP_BINARY_NAME="myapp"

# Dispatch — add app-specific commands before the fallthrough
case "${1:-help}" in
    # import)  go run . import "$2" ;;
    *)  dev_dispatch "$@" ;;
esac
```

The shared template provides: `build`, `test`, `e2e`, `serve`, `codegen`, `lint`, `lint-api`, `ci`, `provision`, `deploy`, `secret`, `docker`, `help`.

Override any command by defining it before the fallthrough:
```sh
    build)  go build -tags desktop -o myapp . ;;  # custom build
    *)      dev_dispatch "$@" ;;                  # everything else
```

**Option B: Standalone (for projects not adjacent to appbase)**

If using the installed `appbase` CLI (no sibling directory), write `./dev` standalone.
Delegates shared operations to `appbase`, handles app-specific commands directly:

```sh
#!/bin/sh
set -e
cd "$(dirname "$0")"

case "${1:-help}" in
    build)      go build -o myapp . ;;
    test)       go test -v -count=1 ./... ;;
    serve)      eval "$(appbase secret env 2>/dev/null)" && go run . serve ;;
    codegen)    appbase codegen ;;
    lint-api)   appbase lint-api ;;
    ci)         appbase lint-api && go vet ./... && go build ./... && go test ./... ;;
    deploy)     appbase deploy ;;
    secret)     shift; appbase secret "$@" ;;
    *)          appbase "$@" ;;  # delegate everything else
esac
```

Make it executable: `chmod +x dev`

### Migrating from `./tc` to `./dev`

If your project uses `./tc` (the old naming convention):
1. `mv tc dev` — rename the script
2. Update any CI/CD scripts that reference `./tc`
3. Optionally, source the shared template to get automatic updates:
   - Replace the inline commands with `. "../appbase/deploy/dev-template.sh"` + `dev_dispatch "$@"`
   - Keep app-specific commands as case entries before the fallthrough

### 9. Copy Deploy Templates

```bash
mkdir deploy
cp ../appbase/deploy/Dockerfile deploy/
cp ../appbase/deploy/docker-compose.yml deploy/
```

Edit the Dockerfile `COPY` and `RUN go build` lines to match your app's structure.

### 10. Add CI

Copy the CI workflow from appbase's `.github/workflows/ci.yml` and adapt.

### 11. Configure Claude Code permissions

Create `.claude/settings.local.json` to avoid repeated approval prompts:

```json
{
  "permissions": {
    "allow": [
      "Bash(go:*)", "Bash(git:*)", "Bash(sh:*)",
      "Bash(appbase:*)", "Bash(gcloud:*)", "Bash(docker:*)",
      "Bash(gh:*)", "Bash(oapi-codegen:*)"
    ]
  }
}
```

See `docs/claude-code-settings.md` in appbase for the full recommended set and explanation.

### 12. Verify

```bash
go build ./...
go test -v ./...
go run . serve    # Test the server — login page at /
go run . list     # Test the CLI
```

## Choosing a Pattern

| Pattern | When to use | Example |
|---------|------------|---------|
| **API-first (recommended)** | Apps with a web UI, CLI client, or external consumers | `examples/todo-api/` (Svelte frontend) |
| **Hand-written routes** | Simple apps, internal tools, prototypes | `examples/todo-store/` |
| **Desktop (Wails)** | Native desktop app, same API | See `examples/todo-api/DESKTOP.md` |

For API-first apps, see the `api-first` skill for the full workflow.

## Key Patterns

- **Three runtime modes** — local CLI (`myapp list` — auto-serve, no login), web server (`myapp serve`), remote CLI (`myapp list --server https://...`). Apps work in all three without changes.
- **Auto-serve CLI** — CLI commands auto-start an ephemeral server if no `--server` flag. No `serve &` needed.
- **Login page is built-in** — `app.LoginPage(handler)` shows Google sign-in when unauthenticated. Skipped in local mode.
- **CLI uses the API** — CLI commands use the generated HTTP client (not direct store access). `appcli.ResolveServerWithAutoServe()` handles server resolution.
- **Desktop via app.Handler()** — use `app.Handler()` with Wails for native desktop apps. Same API, same store, no ports.
- **Secrets in the keychain, not on disk** — `appbase secret set/import` stores in OS keychain. See `docs/secrets.md`.
- **Schema is yours** — appbase manages sessions; you manage everything else via `store.Collection`.
- **Provisioning is one command** — `appbase provision email@example.com` sets up GCP end-to-end.
- **Lint the API pattern** — `appbase lint-api` verifies codegen is up to date.
