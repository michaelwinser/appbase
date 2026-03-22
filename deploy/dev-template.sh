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
                (cd cmd/desktop && wails build)
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

dev_ci() {
    appbase lint-api 2>/dev/null || true
    go vet ./...
    go build ./...
    go test -v -count=1 ./...
    dev_e2e
}

# --- Shared helpers ---

_load_secrets() {
    exports=$(appbase secret env 2>/dev/null || true)
    if [ -n "$exports" ]; then eval "$exports"; return; fi
    if [ -f .env ]; then set -a; . ./.env; set +a; fi
}

# --- Command dispatch ---
# Apps source this file and call dev_dispatch "$@"

dev_dispatch() {
    case "${1:-help}" in
        build)      shift; dev_build "$@" ;;
        test)       dev_test ;;
        e2e)        dev_e2e ;;
        serve)      dev_serve ;;
        codegen)    appbase codegen ;;
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
  codegen            Generate server + client from openapi.yaml
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
