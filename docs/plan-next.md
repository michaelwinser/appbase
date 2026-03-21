# appbase — Next Steps

## Immediate

### 1. Built-in Login Page
- Default handler at `/` that shows Google sign-in when unauthenticated
- After login, redirects to `/` which the app can override with its own content
- The todo example should work end-to-end in a browser with auth

### 2. Deployment & Provisioning
Support four target runtimes: local, local Docker, TrueNAS Docker, Cloud Run.

#### In `./ab` script (for appbase itself):
- `./ab deploy` — deploy to Cloud Run via `gcloud run deploy --source .`
- `./ab provision` — enable GCP APIs, create Firestore DB, configure auth

#### As a reusable module for app scripts:
- Provide shell functions or a Go package that apps import into their `./tc` scripts
- `provision_gcp()` — enables APIs (Cloud Build, Run, Firestore, Artifact Registry)
- `deploy_cloudrun()` — builds and deploys via `--source .`
- `deploy_status()` — shows current deployment

#### Dockerfile template:
- Base Dockerfile in appbase that apps extend
- Multi-stage: dev (with tools) and runtime (minimal)
- Works for both local Docker and Cloud Run

#### docker-compose template:
- For local Docker and TrueNAS Docker
- Mounts SQLite volume, maps ports
- Separate from devcontainer (devcontainer is for development, this is for running)

### 3. Firestore Support in `db` Package
- Currently only SQLite is implemented
- Add Firestore connection using `STORE_TYPE=firestore` and `GOOGLE_CLOUD_PROJECT`
- The `db.DB` type needs to support both SQL and Firestore — may need an interface change
- Session store needs Firestore implementation too

## Later

### 4. Config File Support
- `config.LoadFile("app.yaml")` to read defaults from YAML
- Env vars still override file values

### 5. Secret Manager Integration
- `config.UseSecretManager("gcp")` to resolve secrets from GCP Secret Manager
- Pattern: `${SECRET:key}` in config values

### 6. PostgreSQL Support
- Third store backend
- Connection via `DATABASE_URL` env var

### 7. Forgejo CI
- Alternative to GitHub Actions
- Workflow template for Forgejo
