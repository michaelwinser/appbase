---
name: scaffold-app
description: Create a new application that uses appbase
trigger: When the user wants to create a new app built on the appbase module
---

# Scaffolding a New App on appbase

## What You'll Create

```
myapp/
├── CLAUDE.md              # App-specific AI instructions
├── go.mod                 # Depends on github.com/michaelwinser/appbase
├── main.go                # CLI + server setup using appbase
├── schema.go              # SQL schema for app entities
├── store.go               # Domain store (CRUD using appbase.DB)
├── handler.go             # HTTP handlers
├── usecases_test.go       # Use case tests (UC-XXXX)
├── docs/
│   └── prd.md             # Product requirements with numbered use cases
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

### 2. Create main.go

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
    app, err = appbase.New(appbase.Config{})
    if err != nil {
        return err
    }
    if err := app.Migrate(schema); err != nil {
        return err
    }
    store = &ThingStore{db: app.DB()}
    return nil
}

func main() {
    cli := appcli.New("myapp", "My application", setup)

    cli.SetServeFunc(func() error {
        r := app.Router()
        // Register your routes here
        r.Get("/api/things", listHandler)
        r.Post("/api/things", createHandler)
        return app.Serve()
    })

    // Add CLI commands
    cli.AddCommand(cli.Command("list", "List things", listCmd))

    cli.Execute()
}
```

### 3. Create store.go

Define your domain store using `appbase.DB()`. The store handles CRUD for your entities. Always include `user_id` for multi-tenant queries.

### 4. Create handler.go

HTTP handlers use `appbase.UserID(r)` for auth and `server.RespondJSON`/`server.RespondError` for responses.

### 5. Write the PRD

Create `docs/prd.md` with numbered use cases (UC-XXXX). Each use case has acceptance criteria that map directly to tests.

### 6. Write Use Case Tests

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

### 7. Create the Project Script

Create a `./tc` (or app-specific name) script for common operations:

```sh
#!/bin/sh
set -e
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

case "${1:-help}" in
    build)    go build ./... ;;
    test)     go test -v -count=1 ./... ;;
    serve)    go run . serve ;;
    lint)     go vet ./... ;;
    ci)       go vet ./... && go build ./... && go test -v -count=1 ./... ;;
    deploy)   # Add deployment commands ;;
    help)     echo "Usage: ./tc [build|test|serve|lint|ci|deploy|help]" ;;
    *)        echo "Unknown: $1" >&2; exit 1 ;;
esac
```

Make it executable: `chmod +x tc`

### 8. Add CI

Copy the CI workflow from appbase's `.github/workflows/ci.yml` and adapt.

### 8. Verify

```bash
go build ./...
go test -v ./...
myapp serve    # Test the server
myapp list     # Test the CLI
```

## Key Patterns

- **Auth is automatic** — appbase middleware handles sessions. Use `appbase.UserID(r)` in handlers.
- **Config via env vars** — `PORT`, `STORE_TYPE`, `GOOGLE_CLIENT_ID`, etc. See appbase CLAUDE.md.
- **Schema is yours** — appbase manages sessions table; you manage everything else via `app.Migrate()`.
- **CLI and server share setup** — both call `setup()` which initializes the app and store.
