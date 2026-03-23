# Design: appbase Packaging and Distribution

## Problem

appbase is three things consumed three different ways:

1. **Go module** — runtime library consumed via `go get`
2. **CLI tool** — `appbase` binary consumed via `go install`
3. **Project boilerplate** — devcontainer configs, `./dev` script, deploy templates, CLAUDE.md guidance, Claude Code skills

The first two are properly decoupled — Go's module system handles distribution and versioning. The third is not. Today, apps source `../appbase/deploy/dev-template.sh` directly, copy Dockerfiles by hand, and Claude Code reads `../appbase/` for information. This creates:

- **Peer directory coupling** — apps assume appbase is a sibling directory
- **No versioning** — apps get whatever's on disk, no pinning
- **AI agent confusion** — sessions for one project explore another project's source
- **Manual propagation** — improvements to boilerplate require manually updating each app
- **Drift** — apps modify files that should be appbase-owned, then `appbase update` can't safely refresh them

## Goals

1. An app created with `appbase init` is **self-contained** — no runtime references to `../appbase/`
2. `appbase update` **safely refreshes** appbase-owned files without clobbering app customizations
3. The **boundary between appbase-owned and app-owned** files is clear and enforced
4. AI sessions working on an app have **all the guidance they need** without reading the appbase repo
5. Apps can **extend** the dev tooling with project-specific commands

## Design

### File ownership model

Every file in an appbase project is either **appbase-owned** (managed by `appbase init`/`appbase update`) or **app-owned** (created by the scaffold skill or the developer, never overwritten by appbase).

**Appbase-owned files** (immutable — do not edit):

```
./dev                              # Dev command dispatcher
.devcontainer/
    Dockerfile.frontend            # Base frontend tooling
    docker-compose.yml             # Container orchestration
.claude/
    settings.local.json            # Claude Code permissions
deploy/
    Dockerfile                     # Cloud Run build template
    docker-compose.yml             # Local Docker runtime
```

These files contain a header comment:
```
# Managed by appbase — do not edit. Run 'appbase update' to refresh.
# App-specific commands go in dev-scripts/
```

**App-owned files** (created by scaffold, maintained by developer):

```
CLAUDE.md                          # App-specific AI guidance
app.yaml                          # App config
app.json                          # Deploy compat
openapi.yaml                     # API spec
main.go                          # App entry point
internal/app/                    # Domain code
frontend/                        # Frontend app
dev-scripts/                     # App-specific dev commands
```

### The `./dev` script

`./dev` is appbase-owned. It provides built-in commands and discovers app-specific extensions.

**Built-in commands** (from appbase):

```
./dev serve              # Start the web server
./dev build [target]     # Build (server, desktop, all)
./dev test               # Run Go tests
./dev codegen            # Go server/client + frontend TypeScript types
./dev lint-api           # Verify codegen is up to date
./dev ci                 # Full CI pipeline
./dev frontend <cmd>     # Run command in frontend devcontainer
./dev provision <email>  # GCP setup
./dev deploy             # Cloud Run deployment
./dev secret <cmd>       # Manage secrets
./dev docker <cmd>       # Local Docker
./dev help               # Show all commands
```

**App-specific extensions** via `dev-scripts/`:

```
dev-scripts/
    import.sh            # ./dev import <file>
    seed.sh              # ./dev seed
    reset-db.sh          # ./dev reset-db
```

Discovery: `./dev <name>` checks `dev-scripts/<name>.sh` before falling back to built-ins. Each script is sourced (has access to helper functions) and receives remaining args.

Example `dev-scripts/import.sh`:
```sh
# ./dev import <file> — import travel data from iCal
dev_import() {
    go run . import "$@"
}
```

Convention: define a function named `dev_<command>()`, which `./dev` calls.

### The `appbase` CLI commands

`appbase init` creates a new project:
```
appbase init myapp
```

Creates all appbase-owned files plus default app-owned files (main.go template, empty openapi.yaml, starter CLAUDE.md). The app-owned defaults are a fill-in-the-blanks canvas that the scaffold skill or the developer customizes.

`appbase update` refreshes appbase-owned files:
```
appbase update
```

Overwrites all appbase-owned files from the templates embedded in the CLI binary. Safe because apps don't modify these files. Prints a diff summary of what changed.

Skips app-owned files entirely. If a new appbase version adds a new app-owned default (e.g., a new section in CLAUDE.md), `appbase update` prints a suggestion but doesn't modify the file.

### Devcontainer lifecycle

The `./dev` script manages container lifecycle — no persistent containers.

```sh
# ./dev frontend <cmd> implementation:
# 1. Start container if not running
# 2. Exec command
# 3. Stop container

# Example: ./dev frontend pnpm install
# Equivalent to:
#   docker compose -f .devcontainer/docker-compose.yml up -d frontend
#   docker compose -f .devcontainer/docker-compose.yml exec frontend sh -c "cd /app && pnpm install"
#   docker compose -f .devcontainer/docker-compose.yml down
```

For commands that benefit from a warm container (interactive dev), `./dev frontend shell` starts an interactive session. The container stops when the shell exits.

App-specific frontend tools are added to `.devcontainer/Dockerfile.frontend`. Since this file is appbase-owned, apps extend it via a `Dockerfile.frontend.local` that `FROM`s the base:

Wait — this creates the drift problem again. Better approach: `.devcontainer/Dockerfile.frontend` is appbase-owned but has a well-defined extension point:

```dockerfile
# Managed by appbase — do not edit.
FROM node:20-alpine
ENV PNPM_HOME="/root/.local/share/pnpm"
ENV PATH="$PNPM_HOME:$PATH"
RUN corepack enable && corepack prepare pnpm@latest --activate
RUN pnpm add -g openapi-typescript
RUN apk add --no-cache curl git bash
WORKDIR /app

# App-specific tools — add packages below this line.
# appbase update preserves everything after this marker.
# --- APP TOOLS ---
```

`appbase update` replaces everything above the marker and preserves everything below it. This lets apps add Tailwind, Playwright, etc. without losing them on update.

### Template embedding

The `appbase` CLI binary embeds all template files using `//go:embed`. No file system references to `../appbase/` at runtime.

```go
//go:embed templates/*
var templates embed.FS
```

Templates directory in the appbase repo:
```
cmd/appbase/templates/
    dev.sh                         # ./dev script
    devcontainer/
        Dockerfile.frontend
        docker-compose.yml
    deploy/
        Dockerfile
        docker-compose.yml
    claude/
        settings.local.json
    defaults/                      # App-owned defaults (init only, not update)
        CLAUDE.md.tmpl
        main.go.tmpl
        openapi.yaml.tmpl
        app.yaml.tmpl
```

Templates in `defaults/` are Go `text/template` files. `appbase init` renders them with the app name, port, etc. `appbase update` never touches these.

### Scaffold skill role

With `appbase init` creating the canvas, the scaffold skill's job simplifies to:

1. Run `appbase init <name>` (creates boilerplate + defaults)
2. Customize the app-owned defaults based on user intent:
   - Fill in entity types in `openapi.yaml` and `store.go`
   - Add CLI commands to `main.go`
   - Update `CLAUDE.md` with app-specific context
   - Add app-specific dev scripts to `dev-scripts/`

The skill no longer needs to know the exact content of devcontainer files, deploy templates, or the `./dev` script — `appbase init` handles all of that.

### What the AI session sees

An AI session working on an app sees:
- **`CLAUDE.md`** — app-specific guidance, including devcontainer rules and current appbase patterns
- **`./dev help`** — lists all available commands (built-in + app extensions)
- **`appbase version`** — confirms what version of appbase tooling is installed

It never needs to read `../appbase/`. All guidance is either in the local `CLAUDE.md` or embedded in the CLI.

### Migration path

1. **Embed templates in CLI** — add `cmd/appbase/templates/`, `appbase init`, `appbase update`
2. **Convert `./dev`** — self-contained script (no sourcing), embedded in CLI
3. **Add extension discovery** — `dev-scripts/*.sh` mechanism
4. **Update scaffold skill** — call `appbase init`, then customize app-owned files
5. **Migrate travel-calendar** — `appbase update` in existing project, remove `../appbase` references from `./dev`
6. **Migrate todo-api example** — same treatment

Steps 1-3 are appbase work. Step 4 is a skill update. Steps 5-6 are app migration.

## Open Questions

- Should `appbase update` auto-run after `go get -u github.com/michaelwinser/appbase`? Or is manual `appbase update` sufficient?
- Should the Dockerfile.frontend marker approach work for all appbase-owned files, or just the Dockerfile? (CLAUDE.md has app-specific content too, but it's app-owned so this doesn't apply.)
- Should `./dev` be a shell script or a Go binary? Shell is simpler and transparent. Go would be cross-platform and could share code with the CLI. Shell is probably right for now.
- How to handle `deploy/Dockerfile` customization? The `APP_PKG` build arg varies per app. Could be a variable in `app.yaml` that `appbase update` reads when rendering the template.
