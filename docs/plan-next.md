# appbase — Next Steps

## Completed

### 1. Built-in Login Page
- Default handler via `app.LoginPage(next)` — shows Google sign-in when unauthenticated
- After login, OAuth callback redirects to `/`
- Todo example uses it end-to-end with auth

### 2. Deployment & Provisioning
- `./ab init` — create/update `app.json` interactively
- `./ab provision <email>` — full GCP lifecycle (project, billing, APIs, resources, OAuth validation)
- `./ab deploy` — deploy to Cloud Run with Firestore
- `./ab docker up|down|logs` — local Docker with SQLite
- `deploy/` directory with reusable shell functions (`config.sh`, `provision.sh`, `cloudrun.sh`, `docker.sh`)
- `app.json` stores project identity (name, gcpProject, region, urls)
- `.env` sourced automatically by `./ab run` and `./ab deploy`
- Deploy scripts have test coverage (`deploy_test.sh`, 21 tests)

### 3. Firestore Support in `db` Package
- `db.DB` holds either `*sql.DB` or `*firestore.Client`
- `db.IsSQL()` and `db.Firestore()` for backend detection
- `Migrate()` is a no-op for Firestore (schemaless)
- Session store has dual backends (`session_sql.go`, `session_firestore.go`)
- Todo example demonstrates dual-backend store pattern (`store.go`, `store_sql.go`, `store_firestore.go`)

## Later

### 4. Entity Store Abstraction (`store/` package)
- Opt-in ORM-like layer that maps entities to SQLite or Firestore automatically
- Apps define structs with `store:` tags, get `Collection[T]` with List/Get/Create/Update/Delete
- Assumes low volume, few users — in-memory sorting, single-field Firestore queries only
- Eliminates the 3-file boilerplate (store.go, store_sql.go, store_firestore.go) per entity
- Raw `db.SQL()` and `db.Firestore()` access remains available for complex cases
- Lives in appbase as `store/` package, not a separate module

### 5. Config File Support
- `config.LoadFile("app.yaml")` to read defaults from YAML
- Env vars still override file values

### 5. Secret Manager Integration
- `config.UseSecretManager("gcp")` to resolve secrets from GCP Secret Manager
- Pattern: `${SECRET:key}` in config values

### 7. PostgreSQL Support
- Third store backend
- Connection via `DATABASE_URL` env var

### 8. Forgejo CI
- Alternative to GitHub Actions
- Workflow template for Forgejo
