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
- Secrets stored in OS keychain; `.env` as CI fallback only
- Deploy scripts have test coverage (`deploy_test.sh`, 21 tests)

### 3. Firestore Support in `db` Package
- `db.DB` holds either `*sql.DB` or `*firestore.Client`
- `db.IsSQL()` and `db.Firestore()` for backend detection
- `Migrate()` is a no-op for Firestore (schemaless)
- Session store has dual backends (`session_sql.go`, `session_firestore.go`)
- Todo example demonstrates dual-backend store pattern (`store.go`, `store_sql.go`, `store_firestore.go`)

### 4. Entity Store Abstraction (`store/` package)
- `store.Collection[T]` with typed CRUD on both SQLite and Firestore
- `store:` tag with `,pk` and `,index` options
- SQL backend auto-generates CREATE TABLE from struct metadata
- Firestore backend uses single-field queries, sorts in memory
- Three examples: todo (raw), todo-store (Collection), bookmarks (Collection)

## Next: Unified Config, Secrets, and Environments

### 5. App Config (`app.yaml`)

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

- `config.Load("app.yaml")` reads file, merges active environment overrides
- `APP_ENV=production` selects environment (default: `local`)
- Layering order: `app.yaml defaults → environment overrides → .env → env vars`
- Replaces `app.json` (superset of its fields)
- Deploy scripts read from `app.yaml` instead of `app.json`

### 6. Secret Management

Secrets stored in OS keychain (macOS Keychain, Linux secret-service, Windows Credential Manager) instead of plaintext `.env`.

**CLI:**
```bash
./ab secret set google-client-id "123456.apps.googleusercontent.com"
./ab secret set google-client-secret "GOCSPX-..."
./ab secret list
./ab secret get google-client-id
./ab secret delete google-client-id
```

**Resolution chain (checked in order):**
1. OS keychain (`appbase/<project-name>/<secret-name>`)
2. `.env` file (fallback for containers, CI — no keychain available)
3. GCP Secret Manager (production)
4. Env var override (highest priority)

**`${secret:name}` syntax in app.yaml:**
- Local: resolved from keychain or .env
- Production: resolved from GCP Secret Manager
- `./ab provision` pushes keychain secrets to GCP Secret Manager
- `./ab deploy` uses Cloud Run `--set-secrets` (native integration, secrets never passed as env var values)

**Go library:** `github.com/zalando/go-keyring` — wraps macOS Keychain, Linux secret-service, Windows Credential Manager.

**Benefits:**
- No plaintext secrets on disk
- `./ab secret set` replaces manual .env editing
- Provisioning can push local secrets to GCP automatically
- CI uses env vars (no keychain), same resolution chain

### 7. Port Management

Ports declared in `app.yaml` instead of external portmanager.

- `app.yaml` is the allocation record
- `./ab init` checks for conflicts across sibling projects
- Docker compose reads port from `app.yaml`
- Portmanager becomes optional / deprecated

## Later

### 8. PostgreSQL Support
- Third store backend
- Connection via `DATABASE_URL` env var

### 9. Forgejo CI
- Alternative to GitHub Actions
- Workflow template for Forgejo
