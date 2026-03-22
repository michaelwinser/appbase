# appbase — Shared Application Infrastructure Module

A Go module providing reusable infrastructure for web applications: database connections, authentication, HTTP server scaffolding, CLI base, and deployment tooling.

## Philosophy

Appbase creates and manages **connections to services** (databases, OAuth providers, the runtime environment). Applications use those connections to do their work. Both CLI tools and web servers get the same benefits.

```
┌─────────────────────────────────────────────────┐
│                  Your App                        │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐      │
│  │ Handlers │  │ CLI Cmds │  │ Services │      │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘      │
│       │              │              │            │
│  ┌────┴──────────────┴──────────────┴────┐      │
│  │         Your Domain Store              │      │
│  │  (implements your entity CRUD)         │      │
│  └────────────────┬──────────────────────┘      │
├───────────────────┼──────────────────────────────┤
│                   │         appbase              │
│  ┌────────────────┴──────────────────────┐      │
│  │            appbase.App                 │      │
│  │  ┌─────────┐ ┌──────┐ ┌───────────┐  │      │
│  │  │   DB    │ │ Auth │ │  Router   │  │      │
│  │  │ SQLite  │ │Google│ │   chi     │  │      │
│  │  │Firestore│ │OAuth │ │  health   │  │      │
│  │  │Postgres │ │cookie│ │  CORS     │  │      │
│  │  └─────────┘ └──────┘ └───────────┘  │      │
│  │  ┌─────────┐ ┌──────────────────┐    │      │
│  │  │Sessions │ │  CLI (Cobra)     │    │      │
│  │  │  CRUD   │ │  serve, migrate  │    │      │
│  │  └─────────┘ └──────────────────┘    │      │
│  └───────────────────────────────────────┘      │
└─────────────────────────────────────────────────┘
```

## Package Structure

```
appbase/
├── CLAUDE.md              # This file — AI session instructions
├── go.mod
├── app.go                 # App type — the central coordinator
├── db/                    # Database connections and migration
│   ├── db.go              # DB interface + factory
│   ├── sqlite.go          # SQLite connection + migration runner
│   └── firestore.go       # Firestore connection
├── auth/                  # Authentication
│   ├── google.go          # Google OAuth flow
│   ├── login.go           # Built-in login page
│   ├── session.go         # Session entity + store interface
│   ├── session_sql.go     # SQL session backend
│   ├── session_firestore.go # Firestore session backend
│   └── middleware.go      # HTTP auth middleware
├── server/                # HTTP server
│   ├── server.go          # Router setup, health, CORS
│   └── respond.go         # JSON response helpers
├── config/                # Configuration
│   └── config.go          # Layered config: env vars → defaults (future: files, secrets)
├── cli/                   # CLI base
│   ├── cli.go             # Cobra root command, serve/version, login/logout/whoami
│   ├── auth.go            # Browser OAuth login, authenticated HTTP client
│   └── autoserve.go       # Auto-start ephemeral server for CLI commands
├── .devcontainer/         # Development containers
│   ├── devcontainer.json  # VS Code / Codespaces config
│   ├── docker-compose.yml # workspace (Go) + frontend (Node) services
│   ├── Dockerfile.workspace  # Go + SQLite + oapi-codegen
│   └── Dockerfile.frontend   # Node + pnpm + openapi-typescript
├── deploy/                # Deployment tooling
│   ├── deploy.sh          # Entry point — sources all below
│   ├── config.sh          # app.json reader functions
│   ├── provision.sh       # GCP provisioning (project, billing, APIs, OAuth)
│   ├── cloudrun.sh        # Cloud Run deployment
│   ├── docker.sh          # Local/TrueNAS Docker deployment
│   ├── Dockerfile         # Multi-stage build template
│   ├── docker-compose.yml # Runtime compose template
│   └── deploy_test.sh     # Tests for config/URL functions
├── cmd/appbase/           # Installable CLI (go install ...cmd/appbase@latest)
├── cmd/secret/            # Legacy secret CLI (use cmd/appbase instead)
├── examples/              # Example apps (progression of patterns)
│   ├── todo/              # Raw db connections, hand-written routes
│   ├── todo-store/        # store.Collection, hand-written routes
│   ├── todo-api/          # Full stack: OpenAPI, Svelte frontend, CLI auth, auto-serve
│   └── bookmarks/         # store.Collection, richer entity
├── app.json               # Project identity (name, gcpProject, region, urls)
├── Dockerfile             # Cloud Run build (builds todo example)
└── hyrums/                # Consumer contract tests
    └── README.md          # How apps add tests here
```

## How To Use appbase

### 1. Import and initialize (API-first pattern, recommended)

```go
// Define API in openapi.yaml, generate with: ./tc codegen
// Then implement the generated ServerInterface:

app, _ := appbase.New(appbase.Config{Name: "my-app"})
myServer := &MyServer{store: myStore}
api.HandlerFromMux(myServer, app.Server().Router())  // generated routes
app.Serve()
```

For simpler apps without OpenAPI, hand-write routes:
```go
app.Router().Get("/api/things", myHandler)
```

See `examples/todo-api/` for the full API-first pattern, `examples/todo/` for the simple pattern.

### 2. Use the database

Apps define stores with a backend interface to support both SQLite and Firestore:

```go
// Define a backend interface for your entity
type thingBackend interface {
    List(userID string) ([]Thing, error)
    Create(thing *Thing) error
}

// Factory picks the right backend based on STORE_TYPE
func NewThingStore(d *db.DB) *ThingStore {
    if d.IsSQL() {
        return &ThingStore{backend: &sqlThingBackend{db: d}}
    }
    return &ThingStore{backend: &firestoreThingBackend{db: d}}
}
```

See `examples/todo/store.go`, `store_sql.go`, `store_firestore.go` for a complete example.

### 3. Use auth and the login page

```go
// Built-in login page: shows Google sign-in when unauthenticated
r.Get("/", app.LoginPage(myContentHandler))

// Auth middleware is auto-registered. Access the user in handlers:
func myHandler(w http.ResponseWriter, r *http.Request) {
    userID := appbase.UserID(r)  // from session cookie
    email := appbase.Email(r)
}
```

### 4. Build a CLI (API-first)

CLI commands use the generated HTTP client, not direct store access:

```go
// Built-in: login, logout, whoami, --server flag, auto-serve
// Your commands use the generated client:
serverURL, cleanup, _ := appcli.ResolveServerWithAutoServe(cmd, "myapp")
defer cleanup()
client, _ := api.NewClientWithResponses(serverURL, api.WithHTTPClient(httpClient))
```

**Three runtime modes (automatic):**
- `myapp list` — local mode: auto-starts ephemeral server, no login needed
- `myapp serve` — web server: persistent HTTP server with full OAuth
- `myapp list --server https://prod.app` — remote: uses keychain session

See `cli/auth.go` and `cli/autoserve.go`. Desktop mode via `app.Handler()` with Wails (see `examples/todo-api/DESKTOP.md`).

### 5. OpenAPI codegen

```bash
./tc codegen    # generates api/server.gen.go + api/client.gen.go
./tc lint-api   # verifies codegen is up to date, no hand-written routes
```

## Project Config and Deployment

### app.json

Every project has an `app.json` at the repo root:
```json
{
  "name": "my-app",
  "gcpProject": "my-gcp-project",
  "region": "us-central1",
  "urls": ["http://localhost:3000"]
}
```

Create with `./ab init`. Deploy scripts read from this file.

### Secrets

Secrets are stored in the OS keychain (never as plaintext on disk):
```bash
./ab secret import ~/Downloads/client_secret_*.json  # import from Google
./ab secret set <name> <value>                       # set individually
./ab secret list                                     # show all secrets
```

`./ab run` reads from keychain automatically. `./ab deploy` pushes to GCP Secret Manager.
`.env` is a fallback for CI/containers without a keychain. See `docs/secrets.md`.

### Deployment targets

| Target | Store | Command |
|--------|-------|---------|
| Local | SQLite | `./ab run serve` |
| Local Docker | SQLite | `./ab docker up` |
| Cloud Run | Firestore | `./ab deploy` |

### Provisioning

`./ab provision user@example.com` — creates GCP project, enables APIs, creates Firestore DB, validates OAuth credentials. Reads name/project from `app.json`.

## For AI Sessions (Claude Code)

When working on appbase:

1. **This is a library, not an app** — changes affect all dependent apps
2. **Backward compatibility matters** — don't change exported function signatures without considering consumers
3. **The todo example must always work** — it's the integration test
4. **Run `go test ./...` before committing** — every package should have tests
5. **Consumer tests in `hyrums/`** — dependent apps add tests here that validate their assumptions. Don't break them.

When working on an app that uses appbase:

1. **Don't modify appbase directly** — if you need something new, discuss adding it to appbase as a feature
2. **Your domain entities and store are yours** — appbase provides the connection, you provide the CRUD
3. **Use `appbase.UserID(r)` for auth** — don't roll your own session handling
4. **Migrations are yours** — appbase runs them, you write them

## Environment Variables

| Variable | Purpose | Default |
|----------|---------|---------|
| `PORT` | HTTP server port | `3000` |
| `STORE_TYPE` | Database backend (`sqlite`, `firestore`) | `sqlite` |
| `SQLITE_DB_PATH` | SQLite database file path | `data/app.db` |
| `GOOGLE_CLOUD_PROJECT` | GCP project (for Firestore) | — |
| `GOOGLE_CLIENT_ID` | Google OAuth client ID | — |
| `GOOGLE_CLIENT_SECRET` | Google OAuth client secret | — |
| `GOOGLE_REDIRECT_URL` | OAuth callback URL (auto-detected if unset) | — |
| `ALLOWED_USERS` | Comma-separated email allowlist (empty = open) | — |
