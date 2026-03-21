#!/bin/sh
# deploy/cloudrun.sh — Cloud Run deployment
#
# Requires: deploy/config.sh sourced first
# Requires: gcloud CLI authenticated
#
# NOTE: Cloud Run has an ephemeral filesystem. SQLite data is lost on
# every cold start or scale event. Use STORE_TYPE=firestore for
# production Cloud Run deployments. SQLite is only suitable for
# testing the deploy pipeline.

# deploy_cloudrun — build and deploy to Cloud Run.
# After deployment, captures the service URL and adds it to app.json.
# Usage: deploy_cloudrun <service-name> <project-id> [region]
deploy_cloudrun() {
    service="$1"
    project="$2"
    region="${3:-us-central1}"

    if [ -z "$service" ] || [ -z "$project" ]; then
        echo "Usage: deploy_cloudrun <service-name> <project-id> [region]" >&2
        return 1
    fi

    store_type="${STORE_TYPE:-firestore}"

    echo "Deploying $service to Cloud Run (project: $project, region: $region, store: $store_type)..."

    if [ "$store_type" = "sqlite" ]; then
        echo ""
        echo "  WARNING: Using SQLite on Cloud Run. Data will be lost on cold starts."
        echo "  Use STORE_TYPE=firestore (default) for production."
        echo ""
    fi

    # Build env vars — always include store config
    env_vars="STORE_TYPE=$store_type,GOOGLE_CLOUD_PROJECT=$project"

    # Pass OAuth credentials if set (from .env or environment)
    if [ -n "$GOOGLE_CLIENT_ID" ]; then
        env_vars="${env_vars},GOOGLE_CLIENT_ID=$GOOGLE_CLIENT_ID"
    fi
    if [ -n "$GOOGLE_CLIENT_SECRET" ]; then
        env_vars="${env_vars},GOOGLE_CLIENT_SECRET=$GOOGLE_CLIENT_SECRET"
    fi
    if [ -n "$GOOGLE_REDIRECT_URL" ]; then
        env_vars="${env_vars},GOOGLE_REDIRECT_URL=$GOOGLE_REDIRECT_URL"
    fi
    if [ -n "$ALLOWED_USERS" ]; then
        env_vars="${env_vars},ALLOWED_USERS=$ALLOWED_USERS"
    fi

    gcloud run deploy "$service" \
        --source . \
        --project="$project" \
        --region="$region" \
        --allow-unauthenticated \
        --clear-base-image \
        --set-env-vars="$env_vars"

    # Capture the service URL and add to app.json
    service_url=$(gcloud run services describe "$service" \
        --project="$project" \
        --region="$region" \
        --format="value(status.url)" 2>/dev/null || true)

    if [ -n "$service_url" ]; then
        echo ""
        echo "Service URL: $service_url"

        if [ -f app.json ] && ! grep -q "\"${service_url}\"" app.json 2>/dev/null; then
            _add_app_url "$service_url"
            echo "Added $service_url to app.json urls."
            echo ""
            echo "IMPORTANT: Add this redirect URI to your OAuth client:"
            echo "  ${service_url}/api/auth/callback"
            echo ""
            echo "  Console: https://console.cloud.google.com/apis/credentials?project=$project"
        fi
    fi
}

# deploy_cloudrun_status — show current Cloud Run deployment info.
# Usage: deploy_cloudrun_status <service-name> <project-id> [region]
deploy_cloudrun_status() {
    service="$1"
    project="$2"
    region="${3:-us-central1}"

    if [ -z "$service" ] || [ -z "$project" ]; then
        echo "Usage: deploy_cloudrun_status <service-name> <project-id> [region]" >&2
        return 1
    fi

    gcloud run services describe "$service" \
        --project="$project" \
        --region="$region" \
        --format="table(status.url, status.conditions.status, metadata.annotations.'run.googleapis.com/lastModifier')"
}
