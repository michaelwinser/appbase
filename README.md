# appbase

Shared infrastructure module for Go web applications: database, authentication, HTTP server, CLI, and deployment tooling.

Build an app once, run it three ways — web server with Google OAuth, local CLI with SQLite, native desktop with Wails — all through the same HTTP handler.

## Quick Start

### New project

```bash
# Prerequisites: install mise (https://mise.jdx.dev), then:
go install github.com/michaelwinser/appbase/cmd/appbase@latest
mkdir myapp && cd myapp
appbase init                    # creates app.yaml, app.json
mise install                    # Go, Node, pnpm, codegen tools
```

### Existing project

```bash
cd myapp
mise install                    # set up toolchain
./dev serve                     # start the web server
./dev test                      # run tests
```

## What appbase provides

```
┌─────────────────────────────────────────────┐
│               Your App                       │
│  Handlers  │  CLI Commands  │  Stores        │
├─────────────────────────────────────────────┤
│               appbase                        │
│  DB (SQLite/Firestore)  │  Auth (Google)     │
│  Router (chi)           │  Sessions          │
│  CLI (Cobra)            │  Config (YAML)     │
│  Deploy (Cloud Run)     │  Store (generic)   │
└─────────────────────────────────────────────┘
```

**You write:** domain entities, store CRUD, API handlers, frontend.
**appbase provides:** database connections, OAuth login, session management, server scaffolding, CLI framework, deployment scripts.

## Three runtime modes

Every appbase app runs in three modes automatically:

| Mode | Command | Auth | Database |
|------|---------|------|----------|
| **Web server** | `./dev serve` | Google OAuth | SQLite or Firestore |
| **Local CLI** | `myapp list` | Automatic (dev@localhost) | SQLite at ~/.config/myapp/ |
| **Remote CLI** | `myapp --server URL list` | Keychain session | Server's database |

No code changes between modes — identity injection happens at the transport layer.

## The ./dev script

Every project gets a `./dev` script that wraps common tasks:

```bash
./dev serve              # Start web server (loads secrets from keychain)
./dev build              # Build server binary (+ desktop if cmd/desktop/ exists)
./dev test               # Run Go tests
./dev codegen            # Generate Go server/client + frontend types from openapi.yaml
./dev lint-api           # Verify codegen is up to date
./dev deploy             # Deploy to Cloud Run
./dev provision email    # Full GCP project setup
./dev secret set K V     # Store secret in OS keychain
```

The script sources shared functions from the appbase binary:

```sh
eval "$(appbase dev-template)"
```

## API-first development

The recommended pattern uses OpenAPI as the source of truth:

1. **Define** endpoints in `openapi.yaml`
2. **Generate** Go server interface + client: `./dev codegen`
3. **Implement** the generated interface in `server.go`
4. **Use** the generated client in CLI commands and tests

```go
// Generated interface — implement this
type ServerInterface interface {
    ListTodos(w http.ResponseWriter, r *http.Request)
    CreateTodo(w http.ResponseWriter, r *http.Request)
}

// Your implementation
func (s *TodoServer) ListTodos(w http.ResponseWriter, r *http.Request) {
    userID := appbase.UserID(r)  // from session/transport
    items, _ := s.Store.List(userID)
    server.RespondJSON(w, http.StatusOK, items)
}
```

## Entity persistence

Use `store.Collection[T]` for automatic SQLite + Firestore support:

```go
type Todo struct {
    ID     string `store:"id,pk"`
    UserID string `store:"user_id,index"`
    Title  string `store:"title"`
    Done   bool   `store:"done"`
}

coll, _ := store.NewCollection[Todo](db, "todos")
coll.Create(&todo)
coll.Where("user_id", "==", uid).OrderBy("created_at", store.Desc).All()
```

Tables and indexes are auto-created. New fields added to the struct are auto-migrated on existing databases.

## Authentication

```go
// Built-in login page (shows Google sign-in when unauthenticated)
r.Get("/*", app.LoginPage(myHandler))

// In any handler:
userID := appbase.UserID(r)
email := appbase.Email(r)
token := appbase.AccessToken(r)  // OAuth token for Google API calls
```

For apps that need Google API access (Tasks, Calendar, Drive), add scopes and the required GCP API in `app.yaml`:

```yaml
auth:
  extra_scopes:
    - https://www.googleapis.com/auth/tasks

gcp:
  apis:
    - tasks.googleapis.com
```

The `extra_scopes` are requested during OAuth login. The `gcp.apis` are enabled by `./dev provision`.

## CLI commands

CLI commands use the generated HTTP client, never direct store access:

```go
httpClient, baseURL, cleanup, _ := appcli.ClientForCommand(cmd, "myapp", app.Handler())
defer cleanup()
client, _ := api.NewClientWithResponses(baseURL, api.WithHTTPClient(httpClient))
resp, _ := client.ListTodosWithResponse(ctx)
```

Flags: `--server URL` for remote, `--local` to force local mode, `--data PATH` for custom data directory.

## Testing

Set `APPBASE_TEST_MODE=true` and use `X-Test-User` header — no OAuth or session setup needed:

```go
func setupTestApp(t *testing.T) http.Handler {
    os.Setenv("APPBASE_TEST_MODE", "true")
    app, _ := appbase.New(appbase.Config{Name: "myapp", Quiet: true})
    // ... register routes ...
    return app.Server().Router()
}

h.Run("UC-001", "List todos", func(c *harness.Client) {
    c.SetHeader("X-Test-User", "test@example.com")
    resp := c.GET("/api/todos")
    c.AssertStatus(resp, 200)
})
```

## Project structure

```
myapp/
├── .mise.toml             # Toolchain (Go, Node, pnpm, codegen)
├── app.yaml               # Config (name, port, auth, environments)
├── openapi.yaml           # API spec (source of truth)
├── main.go                # Server + CLI entry point
├── internal/app/
│   ├── store.go           # Entity + store.Collection
│   └── server.go          # Implements generated ServerInterface
├── api/                   # Generated (never edit)
│   ├── server.gen.go
│   └── client.gen.go
├── frontend/              # Svelte SPA
│   └── src/lib/api.ts     # Typed fetch wrappers
├── usecases_test.go       # API-level tests
├── dev                    # Project commands (eval "$(appbase dev-template)")
├── sandbox                # nono sandbox for AI sessions
└── cmd/desktop/           # Wails desktop entry point (optional)
```

## Toolchain

Uses [mise](https://mise.jdx.dev) for per-project tool management:

```bash
mise install       # installs Go, Node, pnpm, oapi-codegen, wails
```

## Sandboxing

For AI-assisted development with Claude Code, the `./sandbox` script provides nono-based isolation:

```bash
./sandbox claude           # Claude Code session with project-only access
appbase sandbox-template   # generate a starter sandbox script
```

## Deployment

```bash
./dev provision user@example.com   # GCP project, APIs, Firestore, OAuth
./dev secret import creds.json     # OAuth credentials to keychain
./dev deploy                       # Cloud Run deployment
```

Provisioning enables 5 infrastructure APIs automatically. App-specific APIs (Tasks, Calendar, etc.) are declared in `app.yaml` under `gcp.apis` and enabled alongside them.

Secrets flow: OS keychain (local) -> GCP Secret Manager (Cloud Run). Never stored as plaintext on disk.

## Examples

| Example | Pattern | What it demonstrates |
|---------|---------|---------------------|
| `examples/todo/` | Raw DB | Hand-written routes, direct SQL |
| `examples/todo-store/` | store.Collection | Generic persistence, hand-written routes |
| `examples/todo-api/` | **Full stack** | OpenAPI, codegen, Svelte, Wails, Google Tasks sync, CLI |
| `examples/bookmarks/` | store.Collection | Richer entity with more fields |

`examples/todo-api/` is the reference implementation. Start there.

## Environment variables

| Variable | Purpose | Default |
|----------|---------|---------|
| `PORT` | HTTP server port | `3000` |
| `STORE_TYPE` | `sqlite` or `firestore` | `sqlite` |
| `SQLITE_DB_PATH` | SQLite file path | `data/app.db` |
| `GOOGLE_CLIENT_ID` | OAuth client ID | from app.yaml secrets |
| `GOOGLE_CLIENT_SECRET` | OAuth client secret | from app.yaml secrets |
| `APPBASE_TEST_MODE` | Enable X-Test-User auth | `false` |

## Claude Code skills

When working on appbase or an appbase app, these slash commands are available:

| Skill | When to use |
|-------|------------|
| `/scaffold-app` | Create a new app from scratch |
| `/api-first` | Add or modify API endpoints |
| `/add-entity` | Add a new domain entity |
| `/review-arch` | Validate against architectural invariants |
| `/deploy` | Provision and deploy |
| `/run-tests` | Run and interpret tests |
