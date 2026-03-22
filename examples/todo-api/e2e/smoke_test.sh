#!/bin/sh
# E2E smoke test for the todo-api example.
#
# Tests the full stack: server, API, CLI, auth — using a disposable database.
#
# Usage:
#   sh examples/todo-api/e2e/smoke_test.sh
#
# Requires: appbase CLI installed (go install ./cmd/appbase)

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
APP_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
REPO_DIR="$(cd "$APP_DIR/../.." && pwd)"

# Use a temp database and a high port unlikely to conflict
DB="/tmp/todo-api-e2e-$$.db"
TEST_PORT=19876
PASS=0
FAIL=0

cleanup() {
    if [ -n "$SERVER_PID" ]; then
        kill "$SERVER_PID" 2>/dev/null || true
        wait "$SERVER_PID" 2>/dev/null || true
    fi
    rm -f "$DB"
    echo ""
    total=$((PASS + FAIL))
    if [ "$FAIL" -eq 0 ]; then
        echo "E2E: $PASS/$total passed"
    else
        echo "E2E: $FAIL/$total FAILED"
        exit 1
    fi
}
trap cleanup EXIT

assert_eq() {
    label="$1"; expected="$2"; actual="$3"
    if [ "$expected" = "$actual" ]; then
        PASS=$((PASS + 1))
        echo "  PASS: $label"
    else
        FAIL=$((FAIL + 1))
        echo "  FAIL: $label (expected '$expected', got '$actual')"
    fi
}

assert_contains() {
    label="$1"; haystack="$2"; needle="$3"
    if echo "$haystack" | grep -q "$needle"; then
        PASS=$((PASS + 1))
        echo "  PASS: $label"
    else
        FAIL=$((FAIL + 1))
        echo "  FAIL: $label (expected to contain '$needle')"
    fi
}

echo "=== todo-api E2E smoke test ==="
echo "  DB: $DB"
echo ""

# Build the app
echo "Building..."
cd "$APP_DIR"
go build -o /tmp/todo-api-e2e-$$ . 2>/dev/null
APP="/tmp/todo-api-e2e-$$"

# Start the server with dev auth
echo "Starting server..."
AUTH_MODE=dev SQLITE_DB_PATH="$DB" PORT="$TEST_PORT" "$APP" serve > /tmp/todo-api-e2e-$$.log 2>&1 &
SERVER_PID=$!

# Wait for server to be ready
BASE="http://localhost:$TEST_PORT"
for i in 1 2 3 4 5 6 7 8 9 10; do
    if curl -s "$BASE/health" >/dev/null 2>&1; then
        break
    fi
    sleep 0.5
done
if ! curl -s "$BASE/health" >/dev/null 2>&1; then
    echo "FAIL: Server didn't start"
    cat /tmp/todo-api-e2e-$$.log
    exit 1
fi
echo "  Server running on $BASE"
echo ""

# Cookie jar for session persistence
COOKIES="/tmp/todo-api-e2e-cookies-$$"
CURL="curl -s -b $COOKIES -c $COOKIES"

# Test: health endpoint
echo "Health check..."
health=$($CURL "$BASE/health")
assert_contains "health returns ok" "$health" '"ok"'

# Test: auth status (dev mode — first request creates session via cookie)
echo "Auth status..."
auth=$($CURL "$BASE/api/auth/status")
assert_contains "dev auth is active" "$auth" '"loggedIn":true'

# Test: list todos (empty)
echo "List todos (empty)..."
todos=$($CURL "$BASE/api/todos")
assert_eq "empty list" "[]" "$todos"

# Test: create todo
echo "Create todo..."
created=$($CURL -X POST -H "Content-Type: application/json" \
    -d '{"title":"E2E test todo"}' "$BASE/api/todos")
assert_contains "created has title" "$created" '"title":"E2E test todo"'
assert_contains "created has id" "$created" '"id"'

# Extract ID
TODO_ID=$(echo "$created" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)

# Test: list todos (one item)
echo "List todos (one item)..."
todos=$($CURL "$BASE/api/todos")
assert_contains "list has the todo" "$todos" "E2E test todo"

# Test: create with empty title fails
echo "Create with empty title..."
bad=$($CURL -o /dev/null -w "%{http_code}" -X POST -H "Content-Type: application/json" \
    -d '{"title":""}' "$BASE/api/todos")
assert_eq "empty title returns 400" "400" "$bad"

# Test: delete todo
if [ -n "$TODO_ID" ]; then
    echo "Delete todo..."
    del=$($CURL -X DELETE "$BASE/api/todos/$TODO_ID")
    assert_contains "delete returns ok" "$del" '"ok"'

    todos_after=$($CURL "$BASE/api/todos")
    assert_eq "list empty after delete" "[]" "$todos_after"
fi

# Cleanup cookies
rm -f "$COOKIES"

# Cleanup binary
rm -f "$APP" /tmp/todo-api-e2e-$$.log
