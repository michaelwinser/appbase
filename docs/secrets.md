# Secret Management

## Goal

**No plaintext secrets anywhere.** Not in `.env` files, not in git, not in deploy commands, not in the Cloud Run console.

## Where secrets live

| Environment | Storage | How it gets there |
|-------------|---------|-------------------|
| Local dev (host) | OS keychain | `./ab secret set` |
| Docker container | `/run/secrets/<name>` | Docker Compose `secrets:` |
| CI | Environment variables | CI platform secrets config |
| Cloud Run | GCP Secret Manager | Pushed by `./ab deploy` |

The `.env` file exists as a **fallback** for environments without a keychain (CI runners, minimal containers). It is not the primary mechanism.

## Developer workflow

### 1. Provision the project

```bash
./ab provision user@example.com
```

This sets up GCP (project, APIs, Firestore, etc.) and prompts you to create an OAuth client in the Cloud Console.

### 2. Create OAuth client (manual — no public API)

In the Cloud Console:
1. Go to APIs & Services → Credentials
2. Create Credentials → OAuth client ID
3. Application type: Web application
4. Add redirect URIs (provision prints these from app.json)
5. Copy the Client ID and Client Secret

### 3. Import credentials into the OS keychain

Download the credentials JSON from the Cloud Console and import directly:

```bash
./ab secret import ~/Downloads/client_secret_123456789.json
```

This parses the JSON, extracts `client_id` and `client_secret`, and stores both in the keychain. You can then delete the JSON file.

Or set them individually:

```bash
./ab secret set google-client-id "123456789.apps.googleusercontent.com"
./ab secret set google-client-secret "GOCSPX-..."
```

This stores them in:
- **macOS**: Keychain Access (login keychain)
- **Linux**: GNOME Keyring / KDE Wallet (via secret-service)
- **Windows**: Credential Manager

Secrets are keyed by `appbase/<project-name>/<secret-name>`.

### 4. Run locally

```bash
./ab run serve
```

The script reads secrets from the keychain and exports them as environment variables for the process. The app picks them up via `GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET`.

### 5. Deploy to Cloud Run

```bash
./ab deploy
```

This automatically:
1. Reads secrets from the keychain
2. Pushes them to GCP Secret Manager (`google-client-id`, `google-client-secret`)
3. Deploys with `--set-secrets` (Cloud Run mounts them from Secret Manager)
4. Non-secret config (`STORE_TYPE`, `GOOGLE_CLOUD_PROJECT`) goes via `--set-env-vars`

Secrets never appear as plaintext in the deploy command or Cloud Run environment variables.

## CLI reference

```bash
./ab secret set <name> <value>      # Store in OS keychain
./ab secret get <name>              # Retrieve from keychain
./ab secret delete <name>           # Remove from keychain
./ab secret list                    # Show secrets from all sources
./ab secret import <creds.json>     # Import Google OAuth credentials JSON
./ab secret push <name1,name2>      # Push keychain → GCP Secret Manager
```

The project name is read from `app.json` automatically.

## Resolution chain

When the app needs a secret at runtime, the resolution chain is:

1. **OS keychain** — primary store for local development
2. **Docker secrets** — `/run/secrets/<name>` (Docker Compose)
3. **`.env` file** — fallback for CI or environments without a keychain
4. **GCP Secret Manager** — production (Cloud Run)
5. **Environment variable** `SECRET_<NAME>` — highest priority override

The chain stops at the first hit. Environment variables always win.

## Using `${secret:name}` in app.yaml

When `app.yaml` config loading is enabled, secrets can be referenced inline:

```yaml
environments:
  production:
    auth:
      client_id: ${secret:google-client-id}
      client_secret: ${secret:google-client-secret}
```

These are resolved at load time using the resolution chain above.

## Docker Compose secrets

For local Docker deployments, use Docker Compose's native secrets mechanism:

```yaml
# docker-compose.yml
services:
  app:
    secrets:
      - google-client-id
      - google-client-secret
    environment:
      - STORE_TYPE=sqlite

secrets:
  google-client-id:
    environment: GOOGLE_CLIENT_ID
  google-client-secret:
    environment: GOOGLE_CLIENT_SECRET
```

The app reads these from `/run/secrets/<name>` via the `DockerSecretResolver`.

## CI environments

CI runners typically don't have an OS keychain. Use the platform's secrets feature:

- **GitHub Actions**: Repository secrets → environment variables
- **Forgejo**: Repository secrets → environment variables

The `EnvFileResolver` and environment variable override (`SECRET_<NAME>`) cover this case.

## Adding new secrets

To add a new secret to an appbase app:

1. Add it to the keychain: `./ab secret set my-new-secret "value"`
2. Reference it in code via `os.Getenv("MY_NEW_SECRET")` or `${secret:my-new-secret}` in app.yaml
3. Add it to the `env` command in `cmd/secret/main.go` (maps keychain name → env var)
4. Add it to `deploy_cloudrun` in `deploy/cloudrun.sh` (pushes to Secret Manager)
5. Deploy: `./ab deploy`

## Security considerations

- The OS keychain is encrypted at rest by the OS
- GCP Secret Manager encrypts secrets at rest and in transit
- Docker secrets are mounted as tmpfs (memory-only, not written to disk)
- `.env` files are gitignored but are plaintext on disk — use only as a fallback
- The `./ab secret` CLI never logs or prints secret values (except `get`)
- Cloud Run `--set-secrets` mounts secrets as environment variables from Secret Manager — they don't appear in `gcloud run services describe` output
