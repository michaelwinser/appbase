# appbase — Next Steps

## Completed

### 1. Built-in Login Page
- `app.LoginPage(next)` — shows Google sign-in when unauthenticated
- OAuth callback redirects to `/`, todo example works end-to-end

### 2. Deployment & Provisioning
- `./ab init`, `./ab provision`, `./ab deploy`, `./ab docker`
- `deploy/` directory with modular shell functions
- `app.json` for project identity, secrets in OS keychain
- Deploy scripts with test coverage

### 3. Firestore Support
- `db.DB` with dual SQL/Firestore backends, `db.IsSQL()`, preflight check
- Session store with SQL and Firestore backends
- Todo example with dual-backend store pattern

### 4. Entity Store Abstraction (`store/` package)
- `store.Collection[T]` with typed CRUD on both SQLite and Firestore
- `store:` tags, auto-migration, in-memory sorting for Firestore
- Four examples: todo, todo-store, bookmarks, todo-api

### 5. OpenAPI-Driven Development
- `openapi.yaml` as canonical API definition
- `./ab codegen` generates chi server + Go client from spec
- CLI browser-based OAuth login (`login`, `logout`, `whoami` built-in)
- CLI uses generated HTTP client with `--server` flag
- `./ab lint-api` validates codegen is up to date, no hand-written routes
- `api-first` skill guides the workflow
- `examples/todo-api/` demonstrates the full pattern

### 6. Secret Management (partially complete)
- OS keychain, Docker secrets, .env fallback, GCP Secret Manager
- `./ab secret set/get/delete/list/import/push`
- `./ab deploy` pushes secrets to Secret Manager, uses `--set-secrets`
- `docs/secrets.md` documents the full workflow

## Next: Unified Config

### 7. App Config (`app.yaml`)

Replace `app.json` and scattered env vars with a single config file.

```yaml
name: my-app
port: 3000

store:
  type: sqlite
  path: data/app.db

auth:
  allowed_users: []

environments:
  local:
    url: http://localhost:3000

  docker:
    url: http://localhost:${PORT}

  production:
    url: https://my-app-abc.run.app
    store:
      type: firestore
      gcp_project: xwind-appbase
    auth:
      client_id: ${secret:google-client-id}
      client_secret: ${secret:google-client-secret}
      allowed_users:
        - admin@example.com
```

Code exists (`config/appconfig.go`) but needs to be wired in:
- `appbase.New()` loads `app.yaml` if present, calls `SetEnvVars()`
- `./ab init` generates `app.yaml` instead of `app.json`
- Deploy scripts read from `app.yaml` (migrate from `app.json`)
- `${secret:name}` resolved via the existing secret chain
- `APP_ENV` selects environment
- Env vars still override everything

### 8. Port Management

Ports declared in `app.yaml` instead of external portmanager.

- `app.yaml` is the allocation record
- `./ab init` checks for conflicts across sibling projects
- Docker compose reads port from `app.yaml`
- Portmanager becomes optional / deprecated

## Next: App Runtime and Desktop

### 9. Auto-Serve CLI

CLI commands that talk to the API currently require a running server:
```sh
./travel serve &
./travel add "Trip" --server http://localhost:3000
```

With auto-serve, the CLI detects no `--server` flag, silently starts an ephemeral
server on a random port, runs the command, and tears down:
```sh
./travel add "Trip"    # just works
```

Implementation:
- `cli/autoserve.go` — start server on `:0`, wait for ready, set server URL
- Detect: no `--server` flag AND not the `serve` command → auto-serve
- Server runs in a goroutine, command runs against `http://localhost:<port>`
- Teardown on command completion
- Optional: keep the server alive for a session (cache PID + port in /tmp)

### 10. Wails Desktop Wrapper

Wrap any appbase app as a native desktop application using Wails.
No ports, no browser — the app runs as a native window.

Pattern (proven in ../projects/electrician):
- Wails `AssetServer.Handler` receives the app's HTTP handler
- Frontend makes standard `fetch("/api/...")` calls — no Wails IPC needed
- Same handler works in web mode (with port) and desktop mode (no port)
- Same auth, same API, same store

What appbase provides:
- `app.Handler()` method returning `http.Handler` for Wails integration
- Template `cmd/desktop/main.go` with Wails setup
- Skill for adding Wails to an existing app
- Dual-mode main: `serve` starts HTTP server, default launches Wails window

Database safety:
- SQLite file locking prevents concurrent access
- Each app has its own `data/app.db` — no cross-app conflicts
- Desktop mode and web mode should not run simultaneously on the same DB

### 11. Traefik Gateway + Wildcard DNS

Replace manual port management with automatic hostname-based routing:
```
travel.dev.local    → Traefik → localhost:random (travel-calendar)
bookmarks.dev.local → Traefik → localhost:random (bookmarks)
travel.nas.local    → Traefik → TrueNAS Docker  (travel-calendar)
```

Components:
- **Traefik** — reverse proxy, routes by hostname, auto-discovers services
- **Wildcard DNS** — `*.dev.local` via dnsmasq or /etc/hosts
- **Service registration** — apps register via Docker labels or file config
- **TLS** — optional, Traefik can handle Let's Encrypt or self-signed

What appbase provides:
- `deploy/traefik/` — Traefik config templates
- `appbase register` — registers an app with the local Traefik instance
- `app.yaml` gains a `hostname` field (e.g., `travel.dev.local`)
- OAuth redirect URIs use hostnames instead of `localhost:PORT`
- Works across targets: local dev, Docker, TrueNAS

This eliminates portmanager entirely and gives every app a stable URL.

## Later

### 12. GitHub Actions CI with Workload Identity Federation
- Zero-secrets CI via WIF
- `./ab ci setup` command

### 13. PostgreSQL Support
- Third store backend via `DATABASE_URL`

### 14. Forgejo CI
- Workflow template for self-hosted CI
