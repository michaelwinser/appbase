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
│   ├── session.go         # Session entity + store
│   └── middleware.go      # HTTP auth middleware
├── server/                # HTTP server
│   ├── server.go          # Router setup, health, CORS
│   └── respond.go         # JSON response helpers
├── config/                # Configuration
│   └── config.go          # Layered config: env vars → defaults (future: files, secrets)
├── cli/                   # CLI base
│   └── cli.go             # Cobra root command, serve/version, app command helper
├── examples/              # Example apps
│   └── todo/              # Complete todo app using all capabilities
│       ├── main.go
│       ├── store.go
│       ├── handler.go
│       └── openapi.yaml
└── hyrums/                # Consumer contract tests
    └── README.md          # How apps add tests here
```

## How To Use appbase

### 1. Import and initialize

```go
import "github.com/michaelwinser/appbase"

func main() {
    app := appbase.New(appbase.Config{
        Name: "my-app",
        // DB auto-configured from STORE_TYPE env var
        // Auth auto-configured from GOOGLE_CLIENT_ID/SECRET env vars
    })
    defer app.Close()

    // Register your routes
    app.Router().Get("/api/things", myHandler)

    // Start serving
    app.Serve()
}
```

### 2. Use the database

```go
// Your app defines its own store using app.DB()
type MyStore struct {
    db appbase.DB
}

func (s *MyStore) CreateThing(thing *Thing) error {
    // Use the connection appbase provides
    return s.db.Exec("INSERT INTO things ...")
}
```

### 3. Use auth

```go
// Auth middleware is auto-registered. Access the user in handlers:
func myHandler(w http.ResponseWriter, r *http.Request) {
    userID := appbase.UserID(r)  // from session cookie
    email := appbase.Email(r)
}
```

### 4. Build a CLI

```go
// Your app adds commands to the base CLI
app := appbase.New(config)
app.CLI().AddCommand(&cobra.Command{
    Use:   "import",
    Short: "Import data from CSV",
    Run: func(cmd *cobra.Command, args []string) {
        // app.DB() is available here too
    },
})
app.CLI().Execute()
```

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
