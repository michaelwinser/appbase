---
name: change-toolchain
description: Add, update, or remove tools in the devcontainer
trigger: When the user wants to change development tools, update Node/Go versions, add a new dev dependency, or modify the build environment
---

# Changing the Development Toolchain

The devcontainer is shared infrastructure. Changes affect all apps that use appbase. Treat toolchain changes with the same care as API changes.

## Devcontainer Structure

```
.devcontainer/
├── devcontainer.json         # VS Code / Codespaces config
├── docker-compose.yml        # Orchestrates workspace + frontend containers
├── Dockerfile.workspace      # Go + SQLite + codegen tools
└── Dockerfile.frontend       # Node + pnpm + OpenAPI TS codegen
```

**Workspace container** (optional — developers can use host Go):
- Go compiler, SQLite libs, oapi-codegen
- Apps rarely need to modify this

**Frontend container** (apps extend this):
- Node.js, pnpm, openapi-typescript
- Apps add their framework: Svelte, React, Playwright, etc.

## Adding a Tool

### To the workspace (Go/backend):

1. Add to `.devcontainer/Dockerfile.workspace`
2. Rebuild: `docker compose -f .devcontainer/docker-compose.yml build workspace`
3. Verify the todo example still builds: `go build ./...` and `go test ./...`
4. Document in CLAUDE.md if it's user-facing

### To the frontend container:

1. Add to `.devcontainer/Dockerfile.frontend`
2. Rebuild: `docker compose -f .devcontainer/docker-compose.yml build frontend`
3. Verify: run the codegen tools to ensure they still work
4. Document in CLAUDE.md if it's user-facing

### App-specific tools (not in appbase):

Apps extend the frontend container in their own Dockerfile:

```dockerfile
FROM ghcr.io/michaelwinser/appbase-frontend:latest
RUN pnpm add -g @sveltejs/kit
RUN apk add --no-cache chromium  # for Playwright
```

This keeps appbase clean and lets apps choose their own framework.

## Updating Versions

### Node.js version:
1. Change `FROM node:XX-alpine` in `Dockerfile.frontend`
2. Test that `pnpm` and `openapi-typescript` still work
3. Note: apps may pin specific Node versions — check before bumping

### Go version:
1. Change `FROM golang:X.XX-alpine` in `Dockerfile.workspace`
2. Update `go.mod` Go version directive
3. Run `go build ./...` and `go test ./...`
4. Note: apps inherit the Go version from their own `go.mod`, not from appbase

### pnpm version:
1. Change `corepack prepare pnpm@X.X.X --activate` in `Dockerfile.frontend`
2. Verify with `pnpm --version` in container

## Removing a Tool

1. Remove from the appropriate Dockerfile
2. Search for usage in appbase: `grep -r "toolname" .`
3. Search for usage in known dependent apps
4. If used by apps but not appbase, move to the app's own Dockerfile
5. Document the removal in the commit message

## Verification Checklist

- [ ] `docker compose -f .devcontainer/docker-compose.yml build` succeeds
- [ ] `go build ./...` in workspace container
- [ ] `go test ./...` passes all tests including todo example
- [ ] Todo example can be built and run
- [ ] CLAUDE.md updated if needed
- [ ] Commit message explains what changed and why

## Principles

1. **Minimal base** — appbase containers should only include tools that ALL apps need
2. **Apps extend, don't modify** — app-specific tools go in the app's Dockerfile
3. **Pin versions** — use specific versions, not `latest`, for reproducibility
4. **Test after changes** — always verify the full test suite after toolchain changes
5. **Document breaking changes** — if an update might affect apps, say so in the commit
