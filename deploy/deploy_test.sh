#!/bin/sh
# deploy/deploy_test.sh — Tests for deploy shell functions
#
# Run: sh deploy/deploy_test.sh
# Or:  ./ab test deploy
#
# Tests the config reading, URL handling, and redirect URI generation
# without requiring GCP credentials or Docker.

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
. "${SCRIPT_DIR}/config.sh"

PASS=0
FAIL=0
TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

# --- test helpers ---

assert_eq() {
    label="$1"
    expected="$2"
    actual="$3"
    if [ "$expected" = "$actual" ]; then
        PASS=$((PASS + 1))
    else
        FAIL=$((FAIL + 1))
        printf "  FAIL: %s\n    expected: %s\n    actual:   %s\n" "$label" "$expected" "$actual"
    fi
}

assert_contains() {
    label="$1"
    haystack="$2"
    needle="$3"
    if echo "$haystack" | grep -q "$needle"; then
        PASS=$((PASS + 1))
    else
        FAIL=$((FAIL + 1))
        printf "  FAIL: %s\n    expected to contain: %s\n    actual: %s\n" "$label" "$needle" "$haystack"
    fi
}

# --- fixtures ---

write_fixture() {
    cat > "$TMPDIR/app.json" <<'JSON'
{
  "name": "test-app",
  "gcpProject": "test-project-123",
  "region": "europe-west1",
  "urls": [
    "http://localhost:3000",
    "https://test-app-abc.run.app"
  ]
}
JSON
}

write_empty_fixture() {
    cat > "$TMPDIR/app-empty.json" <<'JSON'
{
  "name": "bare-app",
  "gcpProject": "",
  "region": "",
  "urls": []
}
JSON
}

write_no_urls_fixture() {
    cat > "$TMPDIR/app-nourls.json" <<'JSON'
{
  "name": "no-urls-app",
  "gcpProject": "proj"
}
JSON
}

# =============================================
# Tests
# =============================================

echo "deploy/config.sh tests"
echo ""

# --- _read_app_json ---

echo "  _read_app_json"

write_fixture
assert_eq "reads name" "test-app" "$(_read_app_json "name" "$TMPDIR/app.json")"
assert_eq "reads gcpProject" "test-project-123" "$(_read_app_json "gcpProject" "$TMPDIR/app.json")"
assert_eq "reads region" "europe-west1" "$(_read_app_json "region" "$TMPDIR/app.json")"
assert_eq "missing field returns empty" "" "$(_read_app_json "nonexistent" "$TMPDIR/app.json")"
assert_eq "missing file returns empty" "" "$(_read_app_json "name" "$TMPDIR/missing.json" 2>/dev/null || echo "")"

# --- app_name, app_gcp_project, app_region ---

echo "  app_name / app_gcp_project / app_region"

write_fixture
assert_eq "app_name" "test-app" "$(app_name "$TMPDIR/app.json")"
assert_eq "app_gcp_project" "test-project-123" "$(app_gcp_project "$TMPDIR/app.json")"
assert_eq "app_region" "europe-west1" "$(app_region "$TMPDIR/app.json")"

write_empty_fixture
assert_eq "app_region defaults to us-central1" "us-central1" "$(app_region "$TMPDIR/app-empty.json")"

# --- _read_app_urls ---

echo "  _read_app_urls"

write_fixture
urls=$(_read_app_urls "$TMPDIR/app.json")
assert_contains "includes localhost" "$urls" "http://localhost:3000"
assert_contains "includes cloud run" "$urls" "https://test-app-abc.run.app"

url_count=$(echo "$urls" | wc -l | tr -d ' ')
assert_eq "url count" "2" "$url_count"

write_empty_fixture
empty_urls=$(_read_app_urls "$TMPDIR/app-empty.json")
assert_eq "empty urls array returns empty" "" "$empty_urls"

# --- app_redirect_uris ---

echo "  app_redirect_uris"

write_fixture
uris=$(app_redirect_uris "$TMPDIR/app.json")
assert_contains "localhost redirect uri" "$uris" "http://localhost:3000/api/auth/callback"
assert_contains "cloud run redirect uri" "$uris" "https://test-app-abc.run.app/api/auth/callback"

uri_count=$(echo "$uris" | wc -l | tr -d ' ')
assert_eq "redirect uri count" "2" "$uri_count"

# --- _add_app_url ---

echo "  _add_app_url"

write_fixture
_add_app_url "https://new-url.example.com" "$TMPDIR/app.json"
new_urls=$(_read_app_urls "$TMPDIR/app.json")
assert_contains "added url is present" "$new_urls" "https://new-url.example.com"

# Idempotent — adding same URL again should not duplicate
_add_app_url "https://new-url.example.com" "$TMPDIR/app.json"
dup_count=$(grep -c "new-url.example.com" "$TMPDIR/app.json")
assert_eq "add_url is idempotent" "1" "$dup_count"

# Original URLs still present
assert_contains "original url preserved" "$(_read_app_urls "$TMPDIR/app.json")" "http://localhost:3000"

# --- no urls field ---

echo "  missing urls field"

write_no_urls_fixture
no_urls=$(_read_app_urls "$TMPDIR/app-nourls.json")
assert_eq "no urls field returns empty" "" "$no_urls"
no_uris=$(app_redirect_uris "$TMPDIR/app-nourls.json")
assert_eq "no urls means no redirect uris" "" "$no_uris"

# =============================================
# Summary
# =============================================

echo ""
total=$((PASS + FAIL))
if [ "$FAIL" -eq 0 ]; then
    echo "OK: $PASS/$total tests passed"
    exit 0
else
    echo "FAIL: $FAIL/$total tests failed"
    exit 1
fi
