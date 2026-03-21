#!/bin/sh
# deploy/deploy.sh — appbase deployment functions (entry point)
#
# Source this in your app's ./tc script to get all deploy functions:
#   DEPLOY_DIR="path/to/appbase/deploy"
#   . "${DEPLOY_DIR}/deploy.sh"
#
# Or from ./ab (which sets SCRIPT_DIR):
#   . "$SCRIPT_DIR/deploy/deploy.sh"
#
# Provides:
#   Config:        app_name, app_gcp_project, app_region, app_redirect_uris
#   Provisioning:  provision_gcp, provision_project, provision_billing,
#                  provision_apis, provision_resources, provision_oauth
#   Cloud Run:     deploy_cloudrun, deploy_cloudrun_status
#   Docker:        deploy_docker_up, deploy_docker_down, deploy_docker_logs
#
# See individual files for documentation.

# Resolve the deploy directory. Callers can set DEPLOY_DIR explicitly,
# or we derive it from SCRIPT_DIR (set by ./ab and ./tc scripts).
if [ -z "$DEPLOY_DIR" ]; then
    if [ -n "$SCRIPT_DIR" ]; then
        DEPLOY_DIR="$SCRIPT_DIR/deploy"
    else
        DEPLOY_DIR="$(cd "$(dirname "$0")" 2>/dev/null && pwd)/deploy"
    fi
fi

. "${DEPLOY_DIR}/config.sh"
. "${DEPLOY_DIR}/provision.sh"
. "${DEPLOY_DIR}/cloudrun.sh"
. "${DEPLOY_DIR}/docker.sh"

# Backward compat alias
deploy_status() { deploy_cloudrun_status "$@"; }
