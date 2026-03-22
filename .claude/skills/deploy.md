---
name: deploy
description: Provision GCP infrastructure and deploy an appbase app to Cloud Run, local Docker, or TrueNAS
trigger: When the user wants to deploy, provision GCP, set up a project for production, or run in Docker
---

# Deploying an appbase Application

## Project Identity — app.json

Every app has an `app.json` at the repo root that stores deployment metadata:

```json
{
  "name": "my-app",
  "gcpProject": "my-gcp-project-id",
  "region": "us-central1",
  "urls": [
    "http://localhost:3000",
    "https://my-app-abc123.run.app"
  ]
}
```

- **name** — Cloud Run service name, login page title
- **gcpProject** — GCP project ID (auto-populated by `./tc provision`)
- **region** — deployment region (default: us-central1)
- **urls** — every base URL where the app is reachable. Used to generate OAuth redirect URIs (`<url>/api/auth/callback`). `deploy_cloudrun` adds the Cloud Run URL here automatically after first deploy.

The `scaffold-app` skill creates this file. For existing apps, create it manually.

## Target Runtimes

| Runtime | Store | How |
|---------|-------|-----|
| Local (host) | SQLite | `go run . serve` |
| Local Docker | SQLite | `./tc docker up` |
| TrueNAS Docker | SQLite | Copy `deploy/docker-compose.yml`, mount volume |
| Cloud Run | Firestore | `./tc deploy` |

## Provisioning GCP (Full Lifecycle)

Use `deploy/deploy.sh` functions. The full provisioning runs 5 steps:

```bash
# From an app's ./tc script:
. "$(dirname "$0")/../appbase/deploy/deploy.sh"

# Full provisioning — creates everything from scratch:
provision_gcp "my-gcp-project" "my-app" "admin@example.com"
```

### What provision_gcp does:

1. **Project creation** (`provision_project`) — `gcloud projects create` if needed
2. **Billing** (`provision_billing`) — links first available billing account (or specify one)
3. **API enablement** (`provision_apis`) — enables Cloud Build, Cloud Run, Firestore, Artifact Registry, Secret Manager, IAP
4. **Resource creation** (`provision_resources`) — creates Firestore database (nam5), Artifact Registry Docker repo
5. **OAuth credentials** (`provision_oauth`) — creates consent screen, validates keychain credentials, prints redirect URIs to configure

### After provisioning:

1. Create OAuth client in Cloud Console (provision prints the URL and steps)
2. Import credentials: `./ab secret import ~/Downloads/client_secret_*.json`
3. Re-run `./ab provision` to verify credentials are valid
4. Deploy with `./ab deploy`

Secrets are stored in the OS keychain, never as plaintext on disk. See `docs/secrets.md`.

### OAuth Redirect URIs

Redirect URIs are derived from the `urls` array in `app.json`:
- Each URL gets `/api/auth/callback` appended
- `http://localhost:3000` → `http://localhost:3000/api/auth/callback`
- `https://my-app-abc123.run.app` → `https://my-app-abc123.run.app/api/auth/callback`

When `deploy_cloudrun` runs for the first time, it captures the Cloud Run service URL, adds it to `app.json` urls, and reminds you to add the new redirect URI to the OAuth client.

## Deploying to Cloud Run

```bash
# Reads name and gcpProject from app.json:
deploy_cloudrun "$(app_name)" "$(app_gcp_project)"

# Or with explicit args:
deploy_cloudrun "my-app" "my-gcp-project" "us-central1"
```

Uses `gcloud run deploy --source .` which builds with Cloud Build and deploys.
After deployment, captures the service URL and adds it to `app.json` if new.

## Running in Docker Locally

The `deploy/` directory provides templates:

- `deploy/Dockerfile` — multi-stage build (builder + minimal runtime)
- `deploy/docker-compose.yml` — runs the app with SQLite volume mount

```bash
docker compose -f deploy/docker-compose.yml up -d --build
```

## For the ./tc Script

Apps should wire these into their project script:

```sh
#!/bin/sh
. "$(dirname "$0")/../appbase/deploy/deploy.sh"

case "${1:-help}" in
    provision) provision_gcp "$(app_gcp_project)" "$(app_name)" "$2" ;;
    deploy)    deploy_cloudrun "$(app_name)" "$(app_gcp_project)" ;;
    status)    deploy_status "$(app_name)" "$(app_gcp_project)" ;;
    docker)    docker compose -f deploy/docker-compose.yml "${2:-up}" ;;
esac
```

## Files

| File | Purpose |
|------|---------|
| `app.json` | Project identity, deployment URLs, OAuth redirect URI source |
| `docs/secrets.md` | Secret management guide |
| `cmd/secret/` | Secret CLI (keychain, import, push to GCP) |
| `deploy/deploy.sh` | Reusable shell functions for provisioning and deployment |
| `deploy/Dockerfile` | Multi-stage build template |
| `deploy/docker-compose.yml` | Local/TrueNAS Docker runtime template |
