#!/bin/sh
# deploy/config.sh — Read project identity from app.json
#
# app.json schema:
#   {
#     "name": "my-app",
#     "gcpProject": "my-gcp-project-id",
#     "region": "us-central1",
#     "urls": [
#       "http://localhost:3000",
#       "https://my-app-abc123.run.app"
#     ]
#   }

# _read_app_json — extract a string field from app.json (no jq dependency).
# Usage: _read_app_json <field> [app_json_path]
_read_app_json() {
    field="$1"
    config="${2:-app.json}"
    if [ ! -f "$config" ]; then
        return 1
    fi
    sed -n "s/.*\"${field}\"[[:space:]]*:[[:space:]]*\"\([^\"]*\)\".*/\1/p" "$config" | head -1
}

# _read_app_urls — extract the urls array from app.json as one URL per line.
# Usage: _read_app_urls [app_json_path]
_read_app_urls() {
    config="${1:-app.json}"
    if [ ! -f "$config" ]; then
        return 1
    fi
    sed -n '/\"urls\"/,/\]/p' "$config" | sed -n 's/.*"\(https\{0,1\}:\/\/[^"]*\)".*/\1/p'
}

# _add_app_url — add a URL to the urls array in app.json if not already present.
# Usage: _add_app_url <url> [app_json_path]
_add_app_url() {
    url="$1"
    config="${2:-app.json}"
    if [ ! -f "$config" ]; then
        return 1
    fi
    # Already present — nothing to do
    if grep -q "\"${url}\"" "$config" 2>/dev/null; then
        return 0
    fi
    # Insert before the last entry's closing bracket
    # Adds a comma after the previous last entry and the new entry before ]
    sed -i'' -e "/\"urls\"/,/\]/ {
        /\]/ i\\
\\    ,\"${url}\"
    }" "$config"
}

# app_name — get the app name from app.json.
app_name() {
    _read_app_json "name" "${1:-app.json}"
}

# app_gcp_project — get the GCP project ID from app.json.
app_gcp_project() {
    _read_app_json "gcpProject" "${1:-app.json}"
}

# app_region — get the deployment region from app.json (default: us-central1).
app_region() {
    region=$(_read_app_json "region" "${1:-app.json}")
    echo "${region:-us-central1}"
}

# app_redirect_uris — generate OAuth redirect URIs from the urls in app.json.
# Appends /api/auth/callback to each base URL.
# Usage: app_redirect_uris [app_json_path]
app_redirect_uris() {
    _read_app_urls "${1:-app.json}" | while IFS= read -r base_url; do
        base_url="${base_url%/}"
        echo "${base_url}/api/auth/callback"
    done
}
