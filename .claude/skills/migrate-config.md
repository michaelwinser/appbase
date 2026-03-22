---
name: migrate-config
description: Migrate an app from app.json to app.yaml
trigger: When the user wants to switch from app.json to app.yaml, or asks about app configuration migration
---

# Migrating from app.json to app.yaml

## Why

`app.yaml` replaces `app.json` as the single source of truth for app config. It adds:
- Environment-specific overrides (local, docker, production)
- `${secret:name}` references resolved from the OS keychain
- Port, store type, and auth config in one place
- Automatic loading by `appbase.New()` at startup

## Quick migration

Run `./tc init` (or `./ab init`). If `app.json` exists, it reads the values as defaults and generates both `app.yaml` and `app.json`.

## Manual migration

Given this `app.json`:

```json
{
  "name": "my-app",
  "gcpProject": "my-project",
  "region": "us-central1",
  "urls": [
    "http://localhost:3000",
    "https://my-app-abc.run.app"
  ]
}
```

Create this `app.yaml`:

```yaml
name: my-app
port: 3000

store:
  type: sqlite
  path: data/app.db

environments:
  local:
    url: http://localhost:3000

  production:
    url: https://my-app-abc.run.app
    store:
      type: firestore
      gcp_project: my-project
    auth:
      client_id: ${secret:google-client-id}
      client_secret: ${secret:google-client-secret}
```

## What to keep

- **Keep `app.json`** — the deploy scripts (`deploy/config.sh`) still read it. Both files can coexist. `app.json` is read by shell scripts, `app.yaml` is read by Go code.
- **Keep `.env`** — still works as a fallback for CI. But for local dev, secrets should be in the OS keychain.

## What changes

| Before | After |
|--------|-------|
| `GOOGLE_CLIENT_ID` in `.env` | `${secret:google-client-id}` in app.yaml, value in keychain |
| `STORE_TYPE` env var | `store.type` in app.yaml |
| `PORT` env var | `port` in app.yaml |
| No environment concept | `APP_ENV=production` selects overrides |

## How it loads

`appbase.New()` checks for `app.yaml`:
1. If found, loads it with the active environment (`APP_ENV`, default: `local`)
2. Resolves `${secret:name}` from: keychain → Docker secrets → .env → GCP Secret Manager
3. Calls `SetEnvVars()` to export values so `db.New()`, auth, and server pick them up
4. Env vars still override everything (highest priority)

If `app.yaml` doesn't exist, everything works as before via env vars.

## Deploy scripts

The deploy scripts in `deploy/` still read `app.json` via `deploy/config.sh`. This is intentional — shell scripts parse JSON more easily than YAML. Both files should have consistent values for `name`, `gcpProject`, and URLs.

When you run `./tc init`, both files are generated together.
