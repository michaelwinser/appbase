#!/bin/sh
# deploy/provision.sh — GCP project provisioning
#
# Requires: deploy/config.sh sourced first
# Requires: gcloud CLI authenticated
#
# Full lifecycle:
#   provision_gcp "my-project" "my-app" "user@example.com"
#
# Individual steps (all idempotent):
#   provision_project "my-project"
#   provision_billing "my-project" [billing-account]
#   provision_apis "my-project"
#   provision_resources "my-project" [region]
#   provision_oauth "my-project" "my-app" "user@example.com"

# provision_project — create a GCP project if it doesn't exist.
provision_project() {
    project="$1"
    if [ -z "$project" ]; then
        echo "Usage: provision_project <project-id>" >&2
        return 1
    fi

    echo "Checking project $project..."
    if gcloud projects describe "$project" >/dev/null 2>&1; then
        echo "  Project $project already exists."
    else
        echo "  Creating project $project..."
        gcloud projects create "$project"
        echo "  Project created."
    fi

    gcloud config set project "$project" 2>/dev/null
}

# provision_billing — link a billing account to the project.
# Auto-detects the first open billing account if none specified.
provision_billing() {
    project="$1"
    billing_account="$2"

    if [ -z "$project" ]; then
        echo "Usage: provision_billing <project-id> [billing-account-id]" >&2
        return 1
    fi

    current=$(gcloud billing projects describe "$project" --format="value(billingAccountName)" 2>/dev/null || true)
    if [ -n "$current" ]; then
        echo "  Billing already linked: $current"
        return 0
    fi

    if [ -z "$billing_account" ]; then
        billing_account=$(gcloud billing accounts list --format="value(name)" --filter="open=true" --limit=1 2>/dev/null || true)
        if [ -z "$billing_account" ]; then
            echo "  ERROR: No billing accounts found. Link one manually:" >&2
            echo "    gcloud billing projects link $project --billing-account=ACCOUNT_ID" >&2
            return 1
        fi
        echo "  Using billing account: $billing_account"
    fi

    echo "  Linking billing account to $project..."
    gcloud billing projects link "$project" --billing-account="$billing_account"
}

# provision_apis — enable all required GCP APIs.
provision_apis() {
    project="$1"
    if [ -z "$project" ]; then
        echo "Usage: provision_apis <project-id>" >&2
        return 1
    fi

    echo "Enabling APIs for $project..."

    apis="
        cloudbuild.googleapis.com
        run.googleapis.com
        firestore.googleapis.com
        artifactregistry.googleapis.com
        secretmanager.googleapis.com
        iap.googleapis.com
        tasks.googleapis.com
    "

    for api in $apis; do
        echo "  Enabling $api..."
        gcloud services enable "$api" --project="$project" 2>/dev/null
    done

    echo "  APIs enabled."
}

# provision_resources — create Firestore DB and Artifact Registry repo.
provision_resources() {
    project="$1"
    region="${2:-us-central1}"

    if [ -z "$project" ]; then
        echo "Usage: provision_resources <project-id> [region]" >&2
        return 1
    fi

    echo "Creating resources in $project..."

    echo "  Creating Firestore database..."
    gcloud firestore databases create \
        --project="$project" \
        --location=nam5 \
        --type=firestore-native 2>/dev/null || echo "  (Firestore database already exists)"

    echo "  Creating Artifact Registry repository for Cloud Run..."
    gcloud artifacts repositories create cloud-run-source-deploy \
        --project="$project" \
        --repository-format=docker \
        --location="$region" \
        --description="Cloud Run source deploy images" 2>/dev/null || echo "  (Repository already exists)"

    echo "  Resources created."
}

# provision_oauth — set up OAuth consent screen and validate credentials.
# Reads redirect URIs from urls in app.json.
#
# Web OAuth clients must be created manually in Cloud Console — there is
# no public API for this. This function:
#   1. Creates the consent screen (via gcloud, idempotent)
#   2. Checks that credentials are in the OS keychain
#   3. Validates the client ID matches the GCP project
#   4. Prints the redirect URIs that must be configured on the client
provision_oauth() {
    project="$1"
    app_name="$2"
    support_email="$3"

    if [ -z "$project" ] || [ -z "$app_name" ] || [ -z "$support_email" ]; then
        echo "Usage: provision_oauth <project-id> <app-name> <support-email>" >&2
        return 1
    fi

    echo "Setting up OAuth for $app_name in $project..."

    # Build redirect URIs from app.json urls
    uri_list=""
    if uris=$(app_redirect_uris 2>/dev/null) && [ -n "$uris" ]; then
        uri_list="$uris"
    else
        uri_list="http://localhost:3000/api/auth/callback"
    fi

    # Step 1: Create consent screen (idempotent)
    echo "  Configuring OAuth consent screen..."
    gcloud iap oauth-brands create \
        --application_title="$app_name" \
        --support_email="$support_email" \
        --project="$project" 2>/dev/null || echo "  (Consent screen already exists)"

    # Step 2: Check OS keychain for credentials
    oauth_ok=true
    project_number=$(gcloud projects describe "$project" --format="value(projectNumber)" 2>/dev/null || true)

    # Try to read client ID from keychain via the secret CLI
    client_id=$(go run ./cmd/secret get "$app_name" google-client-id 2>/dev/null || true)
    client_secret=$(go run ./cmd/secret get "$app_name" google-client-secret 2>/dev/null || true)

    if [ -n "$client_id" ]; then
        echo "  google-client-id found in keychain"

        # Validate: client ID should be {project_number}-xxx.apps.googleusercontent.com
        if [ -n "$project_number" ]; then
            case "$client_id" in
                ${project_number}-*.apps.googleusercontent.com)
                    echo "  Client ID matches project $project (${project_number})"
                    ;;
                *.apps.googleusercontent.com)
                    other_num=$(echo "$client_id" | sed 's/-.*//')
                    echo "  WARNING: Client ID belongs to project number $other_num, not $project ($project_number)"
                    echo "  This may be intentional (shared credentials) or a mistake."
                    ;;
                *)
                    echo "  WARNING: Client ID format is unexpected: $client_id"
                    echo "  Expected: ${project_number}-<hash>.apps.googleusercontent.com"
                    oauth_ok=false
                    ;;
            esac
        fi
    else
        echo "  MISSING: google-client-id not found in keychain"
        oauth_ok=false
    fi

    if [ -n "$client_secret" ]; then
        echo "  google-client-secret found in keychain"
    else
        echo "  MISSING: google-client-secret not found in keychain"
        oauth_ok=false
    fi

    # Step 3: Show redirect URIs that must be on the client
    echo ""
    echo "  Required redirect URIs (from app.json urls):"
    echo "$uri_list" | while IFS= read -r uri; do
        echo "    $uri"
    done

    if [ "$oauth_ok" = false ]; then
        echo ""
        echo "  ACTION REQUIRED:"
        echo ""
        echo "  1. Create a Web OAuth client in Cloud Console:"
        echo "     https://console.cloud.google.com/apis/credentials?project=$project"
        echo ""
        echo "     - Click 'Create Credentials' > 'OAuth client ID'"
        echo "     - Application type: Web application"
        echo "     - Name: ${app_name}-web"
        echo "     - Add the redirect URIs listed above"
        echo ""
        echo "  2. Download the credentials JSON and import:"
        echo "     ./ab secret import ~/Downloads/client_secret_*.json"
        echo ""
        echo "  3. Re-run to verify:"
        echo "     ./ab provision $support_email"
    else
        echo ""
        echo "  OAuth credentials configured."
    fi
    echo ""
}

# provision_gcp — run the full provisioning lifecycle.
provision_gcp() {
    project="$1"
    app_name="$2"
    support_email="$3"
    billing_account="${4:-}"

    if [ -z "$project" ] || [ -z "$app_name" ] || [ -z "$support_email" ]; then
        echo "Usage: provision_gcp <project-id> <app-name> <support-email> [billing-account-id]" >&2
        echo "" >&2
        echo "Example:" >&2
        echo "  provision_gcp my-project my-app user@example.com" >&2
        return 1
    fi

    echo "================================================"
    echo "Provisioning GCP for $app_name"
    echo "  Project: $project"
    echo "  Contact: $support_email"
    echo "================================================"
    echo ""

    echo "[1/5] Project"
    provision_project "$project"
    echo ""

    echo "[2/5] Billing"
    provision_billing "$project" "$billing_account"
    echo ""

    echo "[3/5] APIs"
    provision_apis "$project"
    echo ""

    echo "[4/5] Resources"
    provision_resources "$project"
    echo ""

    echo "[5/5] OAuth"
    provision_oauth "$project" "$app_name" "$support_email"

    echo "================================================"
    echo "Provisioning complete."
    echo ""
    echo "Next steps:"
    echo "  1. If OAuth credentials are missing (see above):"
    echo "     - Create OAuth client in Cloud Console"
    echo "     - ./ab secret import ~/Downloads/client_secret_*.json"
    echo "     - ./ab provision $support_email   (re-run to verify)"
    echo "  2. Deploy: ./ab deploy"
    echo "================================================"
}
