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
#
# Secrets are passed via Cloud Run's --set-secrets flag, which mounts
# them from GCP Secret Manager. They never appear as plaintext env vars
# in the deploy command or Cloud Run console.

# _push_secret_if_needed — ensure a secret exists in GCP Secret Manager.
# Reads from OS keychain first, then .env. Skips if value is empty.
# Usage: _push_secret_if_needed <project> <secret-name> <env-var-name>
_push_secret_if_needed() {
    sm_project="$1"
    secret_name="$2"
    env_var_name="$3"

    # Get value from env (which was sourced from .env or keychain)
    eval "val=\$$env_var_name"
    if [ -z "$val" ]; then
        return 0
    fi

    # Check if secret already exists in Secret Manager
    if gcloud secrets describe "$secret_name" --project="$sm_project" >/dev/null 2>&1; then
        # Add a new version with the current value
        printf '%s' "$val" | gcloud secrets versions add "$secret_name" \
            --project="$sm_project" \
            --data-file=- 2>/dev/null
    else
        # Create the secret
        printf '%s' "$val" | gcloud secrets create "$secret_name" \
            --project="$sm_project" \
            --replication-policy="automatic" \
            --data-file=- 2>/dev/null
    fi
}

# deploy_cloudrun — build and deploy to Cloud Run.
# Pushes secrets to GCP Secret Manager and mounts them via --set-secrets.
# Non-secret config is passed via --set-env-vars.
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

    # Non-secret env vars
    env_vars="STORE_TYPE=$store_type,GOOGLE_CLOUD_PROJECT=$project"
    if [ -n "$ALLOWED_USERS" ]; then
        env_vars="${env_vars},ALLOWED_USERS=$ALLOWED_USERS"
    fi

    # Push secrets to GCP Secret Manager and build --set-secrets flag
    secrets_flag=""
    if [ -n "$GOOGLE_CLIENT_ID" ]; then
        echo "  Pushing google-client-id to Secret Manager..."
        _push_secret_if_needed "$project" "google-client-id" "GOOGLE_CLIENT_ID"
        secrets_flag="GOOGLE_CLIENT_ID=google-client-id:latest"
    fi
    if [ -n "$GOOGLE_CLIENT_SECRET" ]; then
        echo "  Pushing google-client-secret to Secret Manager..."
        _push_secret_if_needed "$project" "google-client-secret" "GOOGLE_CLIENT_SECRET"
        if [ -n "$secrets_flag" ]; then
            secrets_flag="${secrets_flag},GOOGLE_CLIENT_SECRET=google-client-secret:latest"
        else
            secrets_flag="GOOGLE_CLIENT_SECRET=google-client-secret:latest"
        fi
    fi

    # Build the deploy command
    deploy_args="--source . --project=$project --region=$region --allow-unauthenticated --clear-base-image"
    deploy_args="$deploy_args --set-env-vars=$env_vars"

    if [ -n "$secrets_flag" ]; then
        deploy_args="$deploy_args --set-secrets=$secrets_flag"
        echo "  Secrets will be mounted from Secret Manager (not as env vars)."
    fi

    eval gcloud run deploy "$service" $deploy_args

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
