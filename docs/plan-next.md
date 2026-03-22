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

## Later

### 9. GitHub Actions CI with Workload Identity Federation
- Zero-secrets CI via WIF
- `./ab ci setup` command

### 10. PostgreSQL Support
- Third store backend via `DATABASE_URL`

### 11. Forgejo CI
- Workflow template for self-hosted CI
