---
name: scaffold-app
description: Create a new application that uses appbase
trigger: When the user wants to create a new app built on the appbase module
---

# Scaffolding a New App on appbase

## What You'll Create

```
myapp/
├── app.json               # Project identity (name, gcpProject, region)
├── CLAUDE.md              # App-specific AI instructions
├── go.mod                 # Depends on github.com/michaelwinser/appbase
├── main.go                # CLI + server setup using appbase
├── schema.go              # SQL schema for app entities
├── store.go               # Store interface + factory (NewXxxStore)
├── store_sql.go           # SQL backend (SQLite/Postgres)
├── store_firestore.go     # Firestore backend
├── handler.go             # HTTP handlers
├── usecases_test.go       # Use case tests (UC-XXXX)
├── .env                   # Fallback for CI (gitignored, prefer OS keychain)
├── docs/
│   └── prd.md             # Product requirements with numbered use cases
├── deploy/
│   ├── Dockerfile         # Copy from appbase/deploy/Dockerfile, customize
│   └── docker-compose.yml # Copy from appbase/deploy/docker-compose.yml
├── tc                     # Project command script (executable)
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

### 8. Create the Project Script

Create a `./tc` script that wires in appbase deploy functions:

```sh
#!/bin/sh
set -e
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

# Source appbase deploy functions
. "../appbase/deploy/deploy.sh"

# Load secrets from OS keychain (primary) or .env (CI fallback)
_load_secrets() {
    project="$(app_name)"
    exports=$(go run ../appbase/cmd/secret env "$project" 2>/dev/null || true)
    if [ -n "$exports" ]; then eval "$exports"; return; fi
    if [ -f "$SCRIPT_DIR/.env" ]; then set -a; . "$SCRIPT_DIR/.env"; set +a; fi
}

case "${1:-help}" in
    init)       # Create app.json interactively
                printf "App name: "; read -r n
                printf "GCP project: "; read -r p
                cat > app.json <<JSON
{ "name": "$n", "gcpProject": "$p", "region": "us-central1", "urls": ["http://localhost:3000"] }
JSON
                echo "Wrote app.json" ;;
    build)      go build ./... ;;
    test)       go test -v -count=1 ./... ;;
    serve)      _load_secrets && mkdir -p data && go run . serve ;;
    lint)       go vet ./... ;;
    ci)         go vet ./... && go build ./... && go test -v -count=1 ./... ;;
    provision)  provision_gcp "$(app_gcp_project)" "$(app_name)" "$2" ;;
    deploy)     _load_secrets && deploy_cloudrun "$(app_name)" "$(app_gcp_project)" ;;
    secret)     go run ../appbase/cmd/secret "$2" "$(app_name)" ${3:+"$3"} ${4:+"$4"} ;;
    status)     deploy_cloudrun_status "$(app_name)" "$(app_gcp_project)" ;;
    docker)     docker compose -f deploy/docker-compose.yml "${2:-up}" ;;
    help)       echo "Usage: ./tc [init|build|test|serve|lint|ci|provision|deploy|secret|status|docker|help]" ;;
    *)          echo "Unknown: $1" >&2; exit 1 ;;
esac
```

Make it executable: `chmod +x tc`

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
| **API-first (recommended)** | Apps with a web UI, CLI client, or external consumers | `examples/todo-api/` |
| **Hand-written routes** | Simple apps, internal tools, prototypes | `examples/todo-store/` |

For API-first apps, see the `api-first` skill for the full workflow. The key difference: define endpoints in `openapi.yaml` first, then generate server + client code.

## Key Patterns

- **Login page is built-in** — use `app.LoginPage(handler)` on your root route. Shows Google sign-in when unauthenticated, your content when authenticated.
- **Auth is automatic** — appbase middleware handles sessions. Use `appbase.UserID(r)` in handlers.
- **CLI uses the API** — CLI commands should use the generated HTTP client (not direct store access). This ensures the CLI tests the same code path as the web UI. Use `appcli.AuthenticatedClient()` for auth.
- **Secrets in the keychain, not on disk** — `./tc secret set/import` stores in OS keychain. `./tc serve` and `./tc deploy` read from keychain automatically. `.env` is a fallback for CI only. See `docs/secrets.md`.
- **Schema is yours** — appbase manages sessions table; you manage everything else via `store.Collection`.
- **Project identity in app.json** — deploy scripts read name and GCP project from here.
- **Provisioning is one command** — `./tc provision email@example.com` sets up GCP end-to-end.
- **Lint the API pattern** — `./tc lint-api` verifies codegen is up to date and no hand-written routes.
