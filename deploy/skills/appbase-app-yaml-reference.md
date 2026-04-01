---
name: appbase-app-yaml-reference
description: "app.yaml configuration reference — all fields, defaults, secret references, environment overrides. Read this instead of exploring config source code."
trigger: When you need to know what fields app.yaml supports, how to configure auth, store, scopes, tokens, or GCP APIs.
---

# app.yaml Reference

Complete reference for the `app.yaml` configuration file.

## Example

```yaml
name: my-app
port: 3000

store:
  type: sqlite
  path: data/app.db

gcp:
  apis:
    - tasks.googleapis.com
    - calendar-json.googleapis.com

auth:
  tokens:
    dev-token-12345: dev@localhost

environments:
  local:
    url: http://localhost:3000
    auth:
      client_id: ${secret:google-client-id}
      client_secret: ${secret:google-client-secret}
      extra_scopes:
        - https://www.googleapis.com/auth/tasks

  production:
    url: https://my-app.run.app
    store:
      type: firestore
      gcp_project: my-gcp-project
    auth:
      client_id: ${secret:google-client-id}
      client_secret: ${secret:google-client-secret}
      extra_scopes:
        - https://www.googleapis.com/auth/tasks
```

## Top-level fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `name` | string | required | Application name (shown on login page, used for keychain) |
| `port` | int | `3000` | HTTP server port |

## store

Database configuration. Applies to all environments unless overridden.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `store.type` | string | `sqlite` | `sqlite` or `firestore` |
| `store.path` | string | `data/app.db` | SQLite database file path |
| `store.gcp_project` | string | — | GCP project ID (required for Firestore) |

## gcp

GCP-specific configuration for provisioning and deploy.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `gcp.apis` | [string] | — | Additional GCP APIs to enable during `appbase provision` |
| `gcp.scheduler` | [SchedulerJob] | — | Cloud Scheduler jobs to create during `appbase deploy` |

Infrastructure APIs (Cloud Run, Firestore, etc.) are always enabled. The `apis` field is for app-specific APIs like `tasks.googleapis.com` or `calendar-json.googleapis.com`.

### Cloud Scheduler jobs

Declarative Cloud Scheduler HTTP jobs that target Cloud Run endpoints. Created/updated automatically during `appbase deploy`.

```yaml
gcp:
  apis:
    - cloudscheduler.googleapis.com
  scheduler:
    - name: sync-nudge
      schedule: "*/5 * * * *"
      path: /sync/nudge
      method: POST
      headers:
        X-Nudge-Key: my-secret-key
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `name` | string | required | Job name (prefixed with app name: `myapp-sync-nudge`) |
| `schedule` | string | required | Cron expression |
| `path` | string | required | HTTP path on the Cloud Run service |
| `method` | string | `POST` | HTTP method |
| `headers` | map | — | Optional HTTP headers |

Deploy automatically:
1. Creates a `{appname}-scheduler` service account with `roles/run.invoker`
2. Creates each job with OIDC auth targeting the deployed service URL

## auth

Authentication configuration. Can be set at top level or per-environment.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `auth.client_id` | string | — | Google OAuth client ID |
| `auth.client_secret` | string | — | Google OAuth client secret |
| `auth.redirect_url` | string | auto-detected | OAuth callback URL |
| `auth.allowed_users` | [string] | allow all | Email allowlist |
| `auth.extra_scopes` | [string] | — | Additional OAuth scopes to request |
| `auth.tokens` | map[string]string | — | Static token → email mappings |

### Secret references

Use `${secret:name}` to reference secrets from the OS keychain or GCP Secret Manager:

```yaml
auth:
  client_id: ${secret:google-client-id}
  client_secret: ${secret:google-client-secret}
```

Secret resolution order: OS keychain → Docker secrets → `.env` file → GCP Secret Manager → `SECRET_*` env vars.

### Token auth

Static tokens for lightweight auth without OAuth:

```yaml
auth:
  tokens:
    my-dev-token-123: dev@localhost
    ci-test-token-456: ci@test.com
```

Tokens must be at least 8 characters. Configure via env var as alternative: `AUTH_TOKENS=token1=email1,token2=email2`

### Extra scopes

Request additional OAuth permissions for Google API access:

```yaml
auth:
  extra_scopes:
    - https://www.googleapis.com/auth/tasks
    - https://www.googleapis.com/auth/calendar.readonly
```

These are combined with the default scopes (`openid`, `email`, `profile`).

## environments

Per-environment overrides. The active environment is determined by `APP_ENV` (default: `local`).

```yaml
environments:
  local:
    url: http://localhost:3000
    # All auth, store fields can be overridden

  staging:
    url: https://staging.my-app.run.app
    store:
      type: firestore
      gcp_project: my-staging-project

  production:
    url: https://my-app.run.app
    store:
      type: firestore
      gcp_project: my-prod-project
    auth:
      client_id: ${secret:google-client-id}
      client_secret: ${secret:google-client-secret}
      allowed_users:
        - admin@company.com
```

| Field | Type | Description |
|-------|------|-------------|
| `environments.<name>.url` | string | Base URL for this environment |
| `environments.<name>.port` | int | Port override |
| `environments.<name>.store` | StoreConfig | Database override |
| `environments.<name>.auth` | AuthConfig | Auth override |

## Config priority

From highest to lowest:

1. **Environment variables** (`GOOGLE_CLIENT_ID`, `PORT`, etc.)
2. **Explicit `appbase.Config` fields** in Go code
3. **app.yaml** environment-specific overrides
4. **app.yaml** top-level values
5. **Defaults** (port 3000, sqlite, data/app.db)

## Related files

| File | Purpose |
|------|---------|
| `app.yaml` | Runtime configuration (this file) |
| `app.json` | Deploy script compatibility (name, gcpProject, region, urls) |
| `.env` | Fallback for secrets in CI/containers |
