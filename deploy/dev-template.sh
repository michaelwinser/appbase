#!/bin/sh
# appbase ./dev script template
#
# Copy this to your project root as ./dev, then customize.
# Or source the shared functions and add app-specific commands:
#
#   . "$(dirname "$0")/../appbase/deploy/dev-common.sh"
#
# The shared functions in dev-common.sh handle the standard commands.
# Override or extend them by defining your own case entries BEFORE
# the fallthrough to dev_common.

# App-specific settings — customize these
APP_BINARY_NAME="${APP_BINARY_NAME:-$(basename "$(pwd)")}"
APP_E2E_DIR="${APP_E2E_DIR:-e2e}"

# --- Standard commands (override any of these in your ./dev) ---

dev_build() {
    target="${1:-all}"
    case "$target" in
        all)
            echo "Building server..."
            go build -o "$APP_BINARY_NAME" .
            if [ -d cmd/desktop ]; then
                echo "Building desktop..."
                dev_build desktop
            fi
            ;;
        server)
            go build -o "$APP_BINARY_NAME" .
            ;;
        desktop)
            if [ -d cmd/desktop ]; then
                # Copy latest frontend dist and ensure dirs exist
                mkdir -p cmd/desktop/dist cmd/desktop/frontend
                if [ -d frontend/dist ]; then
                    cp -r frontend/dist/* cmd/desktop/dist/ 2>/dev/null || true
                fi
                export PATH="$PATH:$(go env GOPATH)/bin"
                (cd cmd/desktop && wails build -tags desktop -skipbindings)
            else
                echo "No cmd/desktop/ directory found. See examples/todo-api/DESKTOP.md."
                return 1
            fi
            ;;
        *)
            echo "Usage: ./dev build [all|server|desktop]"
            return 1
            ;;
    esac
}

dev_test() {
    go test -v -count=1 ./...
}

dev_e2e() {
    if [ -d "$APP_E2E_DIR" ]; then
        for test in "$APP_E2E_DIR"/*.sh; do
            echo "Running $test..."
            sh "$test"
        done
    else
        echo "No e2e directory found."
    fi
}

dev_serve() {
    _load_secrets
    mkdir -p data
    go run . serve
}

dev_codegen() {
    # Go server + client
    appbase codegen

    # Frontend TypeScript types (if frontend exists)
    if [ -f openapi.yaml ] && [ -d frontend/src/lib ]; then
        echo "Generating frontend types from openapi.yaml..."
        _run_frontend npx openapi-typescript openapi.yaml -o frontend/src/lib/api-types.ts
    fi
}

_run_frontend() {
    # Run a command in the frontend devcontainer if available, otherwise locally.
    # Looks for .devcontainer/ in current dir or parent dirs.
    _dc_file=""
    _search="$(pwd)"
    while [ "$_search" != "/" ]; do
        if [ -f "$_search/.devcontainer/docker-compose.yml" ]; then
            _dc_file="$_search/.devcontainer/docker-compose.yml"
            # Working dir inside container: /app/<relative-path-from-compose-context>
            _rel_path="$(pwd)"
            _rel_path="${_rel_path#"$_search"/}"
            break
        fi
        _search="$(dirname "$_search")"
    done

    if [ -n "$_dc_file" ] && docker compose -f "$_dc_file" ps frontend --status running >/dev/null 2>&1; then
        docker compose -f "$_dc_file" exec -T frontend sh -c "cd /app/$_rel_path && $*"
    elif command -v npx >/dev/null 2>&1; then
        "$@"
    else
        echo "Error: frontend devcontainer not running and npx not found locally."
        echo "Start with: docker compose -f .devcontainer/docker-compose.yml up -d frontend"
        return 1
    fi
}

dev_ci() {
    appbase lint-api 2>/dev/null || true
    go vet ./...
    go build ./...
    go test -v -count=1 ./...
    dev_e2e
}

# --- Shared helpers ---

_ensure_appbase_cli() {
    if command -v appbase >/dev/null 2>&1; then
        return 0
    fi
    # Try GOPATH/bin directly
    _gobin="$(go env GOPATH 2>/dev/null)/bin"
    if [ -x "$_gobin/appbase" ]; then
        export PATH="$PATH:$_gobin"
        return 0
    fi
    echo "Error: appbase CLI not found."
    echo "Install with: go install github.com/michaelwinser/appbase/cmd/appbase@latest"
    echo "Then ensure \$(go env GOPATH)/bin is on your PATH."
    return 1
}

_load_secrets() {
    exports=$(appbase secret env 2>/dev/null || true)
    if [ -n "$exports" ]; then eval "$exports"; return; fi
    if [ -f .env ]; then set -a; . ./.env; set +a; fi
}

# --- Command dispatch ---
# Apps source this file and call dev_dispatch "$@"

dev_dispatch() {
    # Ensure appbase CLI is available for commands that need it
    case "${1:-help}" in
        codegen|lint-api|ci|provision|deploy|secret|docker)
            _ensure_appbase_cli || exit 1
            ;;
    esac

    case "${1:-help}" in
        build)      shift; dev_build "$@" ;;
        test)       dev_test ;;
        e2e)        dev_e2e ;;
        serve)      dev_serve ;;
        codegen)    dev_codegen ;;
        lint)       go vet ./... ;;
        lint-api)   appbase lint-api ;;
        ci)         dev_ci ;;
        provision)  shift; appbase provision "$@" ;;
        deploy)     _load_secrets && appbase deploy ;;
        secret)     shift; appbase secret "$@" ;;
        docker)     appbase docker "${2:-up}" ;;
        help)       dev_help ;;
        *)          echo "Unknown: $1. Run './dev help' for usage." >&2; exit 1 ;;
    esac
}

dev_help() {
    cat <<EOF
$(basename "$(pwd)") — Project Commands

Usage: ./dev <command> [options]

Development:
  build [target]     Build all (or: server, desktop)
  test               Run Go tests
  e2e                Run E2E smoke tests
  serve              Start the web server
  codegen            Generate server + client + frontend types from openapi.yaml
  lint               Run go vet
  lint-api           Verify codegen is up to date
  ci                 Full CI pipeline

Deployment:
  provision <email>  Full GCP setup
  deploy             Deploy to Cloud Run
  secret <cmd>       Manage secrets (set, get, list, import, push)
  docker [up|down]   Local Docker
EOF
}
