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

## Next: API-First with OpenAPI Codegen

### 5. OpenAPI-Driven Development

The OpenAPI spec is the canonical definition of the server API. Everything — server routes, CLI client, web client — is generated from it.

#### What changes

| Concern | Current (hand-written) | API-first (generated) |
|---------|----------------------|----------------------|
| API routes | `r.Get("/api/things", handler)` | Generated chi server from spec |
| Request/response types | Hand-written structs | Generated from spec schemas |
| CLI commands | Call store directly (same process) | HTTP client calling server API |
| CLI auth | Hardcoded "cli-user" | OAuth browser flow → token in keychain |
| Web client | Hand-written `fetch()` calls | Generated TypeScript client |
| API contract | Implicit | Explicit, versioned, testable |

#### The spec

Apps define `openapi.yaml` as their API contract:

```yaml
openapi: 3.0.3
info:
  title: My App
  version: 0.1.0
paths:
  /api/things:
    get:
      operationId: listThings
      security: [session: []]
      responses:
        '200':
          content:
            application/json:
              schema:
                type: array
                items: { $ref: '#/components/schemas/Thing' }
    post:
      operationId: createThing
      security: [session: []]
      requestBody:
        content:
          application/json:
            schema: { $ref: '#/components/schemas/CreateThingRequest' }
      responses:
        '201':
          content:
            application/json:
              schema: { $ref: '#/components/schemas/Thing' }
components:
  schemas:
    Thing:
      type: object
      properties:
        id: { type: string }
        userId: { type: string }
        title: { type: string }
        createdAt: { type: string, format: date-time }
  securitySchemes:
    session:
      type: apiKey
      in: cookie
      name: app_session
```

#### Server codegen (Go)

Uses `oapi-codegen` (already in devcontainer) to generate:

1. **Server interface** — chi-compatible handler interface the app implements:
   ```go
   // Generated: api/server.gen.go
   type ServerInterface interface {
       ListThings(w http.ResponseWriter, r *http.Request)
       CreateThing(w http.ResponseWriter, r *http.Request)
   }
   ```

2. **Types** — request/response structs:
   ```go
   // Generated: api/types.gen.go
   type Thing struct {
       ID        string `json:"id"`
       UserID    string `json:"userId"`
       Title     string `json:"title"`
       CreatedAt string `json:"createdAt"`
   }
   ```

3. **Router wiring** — registers generated routes on chi:
   ```go
   // Generated: api/router.gen.go
   func HandlerFromMux(si ServerInterface, r chi.Router) http.Handler
   ```

The app implements `ServerInterface`:
```go
type Server struct {
    store *ThingStore
}

func (s *Server) ListThings(w http.ResponseWriter, r *http.Request) {
    userID := appbase.UserID(r)
    things, err := s.store.List(userID)
    // ...
}
```

Codegen config (`oapi-codegen.yaml`):
```yaml
package: api
output: api/server.gen.go
generate:
  chi-server: true
  models: true
  embedded-spec: true
```

#### Client codegen (Go — for CLI)

Same `oapi-codegen` generates a typed Go HTTP client:

```go
// Generated: api/client.gen.go
type Client struct {
    Server string
    Client *http.Client
}

func (c *Client) ListThings(ctx context.Context) ([]Thing, error)
func (c *Client) CreateThing(ctx context.Context, body CreateThingRequest) (*Thing, error)
```

Codegen config (`oapi-codegen-client.yaml`):
```yaml
package: api
output: api/client.gen.go
generate:
  client: true
  models: true
```

The CLI uses this client:
```go
func listCmd(cmd *cobra.Command, args []string) error {
    client := api.NewClient(serverURL)
    client.Client = authenticatedHTTPClient() // adds session cookie
    things, err := client.ListThings(ctx)
    // ...
}
```

#### Web client codegen (TypeScript)

Uses `openapi-typescript` (already in devcontainer) to generate types:

```bash
npx openapi-typescript openapi.yaml -o src/lib/api.d.ts
```

Then use `openapi-fetch` for type-safe requests:
```typescript
import createClient from 'openapi-fetch'
import type { paths } from './api'

const client = createClient<paths>({ baseUrl: '' })
const { data } = await client.GET('/api/things')
// data is typed as Thing[]
```

#### CLI authentication (browser flow)

The CLI authenticates the same way as the browser — Google OAuth — but initiated from the terminal:

1. `./myapp login` starts a temporary local HTTP server on a random port
2. Opens the browser to Google OAuth with `redirect_uri=http://localhost:PORT/callback`
3. User approves in browser, Google redirects to localhost callback
4. The CLI captures the auth code, exchanges it for a session
5. Session token stored in OS keychain (reuses existing secret infrastructure)
6. Subsequent CLI commands include the session cookie automatically

New appbase package: `cli/auth.go`

```go
// LoginBrowser opens the browser for Google OAuth and captures the session.
// Stores the session token in the OS keychain.
func LoginBrowser(google *auth.GoogleAuth, appName string) error

// AuthenticatedClient returns an http.Client with the session cookie set.
// Reads the session token from the OS keychain.
func AuthenticatedClient(appName, serverURL string) (*http.Client, error)

// RequireLogin wraps a cobra command to ensure the user is logged in.
func RequireLogin(appName string) cobra.PersistentPreRunE
```

CLI commands:
```bash
./myapp login                        # Browser OAuth → token in keychain
./myapp login --server https://...   # Login to a specific server
./myapp logout                       # Clear token from keychain
./myapp whoami                       # Show current user
./myapp list --server https://...    # Use a specific server
```

#### CLI server URL

The CLI needs to know which server to talk to. Resolution order:

1. `--server` flag (highest priority)
2. `APP_SERVER_URL` env var
3. Active environment URL from `app.json` (e.g., `http://localhost:3000`)
4. Default: `http://localhost:3000`

#### Codegen command

`./ab codegen` (or `./tc codegen`) runs all code generation:

```bash
# In ./ab or ./tc:
cmd_codegen() {
    info "Generating server and client from openapi.yaml..."
    oapi-codegen --config oapi-codegen.yaml openapi.yaml
    oapi-codegen --config oapi-codegen-client.yaml openapi.yaml
    # If frontend exists:
    npx openapi-typescript openapi.yaml -o src/lib/api.d.ts
    success "Codegen complete"
}
```

#### File structure for an API-first app

```
myapp/
├── openapi.yaml                  # API spec (source of truth)
├── oapi-codegen.yaml             # Server codegen config
├── oapi-codegen-client.yaml      # Client codegen config
├── api/
│   ├── server.gen.go             # Generated server interface + router
│   ├── client.gen.go             # Generated Go client (for CLI)
│   └── types.gen.go              # Generated types
├── server.go                     # Implements api.ServerInterface
├── store.go                      # Entity store (same as today)
├── main.go                       # CLI + server wiring
├── app.json
└── frontend/                     # Optional
    └── src/lib/api.d.ts          # Generated TypeScript types
```

#### Implementation steps

1. **`cli/auth.go`** — browser-based OAuth for CLI login, token in keychain
2. **`cli/client.go`** — base HTTP client with auth, server URL resolution
3. **`cli/cli.go`** — add built-in `login`, `logout`, `whoami` commands
4. **`./ab codegen`** command — runs oapi-codegen + openapi-typescript
5. **Todo example (API-first variant)** — `examples/todo-api/` demonstrating the full pattern
6. **Scaffold skill update** — option to scaffold API-first apps
7. **Add-entity skill update** — generates OpenAPI schema + operations, not just store code

#### What appbase provides vs what apps do

| Appbase provides | Apps provide |
|-----------------|-------------|
| `cli/auth.go` — browser OAuth login for CLI | `openapi.yaml` — their API spec |
| `cli/client.go` — authenticated HTTP client | `server.go` — implements ServerInterface |
| Built-in `login/logout/whoami` commands | `store.go` — entity persistence |
| `./ab codegen` tooling | `oapi-codegen.yaml` — codegen config |
| Auth middleware (existing) | Domain logic in handlers |

## Later: Config, Secrets, and Environments

### 6. App Config (`app.yaml`)

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

### 7. Secret Management (partially complete)

Core implemented: OS keychain, Docker secrets, .env fallback, GCP Secret Manager, `./ab secret` CLI.

Remaining:
- `${secret:name}` resolution in app.yaml (needs item 6)
- Docker Compose secrets template

### 8. Port Management

Ports declared in `app.yaml` instead of external portmanager.

- `app.yaml` is the allocation record
- `./ab init` checks for conflicts across sibling projects
- Docker compose reads port from `app.yaml`
- Portmanager becomes optional / deprecated

### 9. GitHub Actions CI with Workload Identity Federation
- Zero-secrets CI: GitHub proves identity directly to GCP (no service account keys)
- `./ab provision` step 6: configure Workload Identity Pool + Provider
- `./ab ci setup` command to create WIF config and output workflow YAML
- Update `.github/workflows/ci.yml` to use `google-github-actions/auth` + `deploy-cloudrun`
- Completes the zero-secrets chain: keychain (dev) → Secret Manager (prod) → WIF (CI auth)

### 10. PostgreSQL Support
- Third store backend
- Connection via `DATABASE_URL` env var

### 11. Forgejo CI
- Alternative to GitHub Actions
- Workflow template for Forgejo
