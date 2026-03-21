---
name: hyrums-test
description: Add or update consumer contract tests that protect dependent apps from appbase changes
trigger: When an app depends on specific appbase behavior, when an appbase update breaks an app, or when the user mentions Hyrum's law or contract tests
---

# Hyrum's Tests — Consumer Contract Tests

## What Are They

Hyrum's Law: "With a sufficient number of users of an API, all observable behaviors of your system will be depended on by somebody."

Hyrum's tests document what apps *actually depend on* in appbase — not just the documented API, but the specific behaviors they rely on. When appbase changes, these tests catch unintended breakage.

See: github.com/michaelwinser/hyrums-tests

## When to Add One

Add a Hyrum's test when:

1. **Your app relies on a specific behavior** that isn't obviously guaranteed by the API
   - "Migrate is idempotent" (safe to call twice)
   - "Sessions with negative TTL are immediately expired"
   - "Empty user_id returns no results, not an error"

2. **An appbase update broke your app** — write the test that would have caught it, submit it to appbase so it doesn't happen again

3. **You're using appbase in a way that might be fragile** — the test documents your assumption so future maintainers know not to break it

## Where They Live

```
appbase/
└── hyrums/
    ├── README.md
    ├── travel_calendar_test.go    # Tests from travel-calendar app
    ├── whereish_test.go           # Tests from whereish app
    └── helpers_test.go            # Shared test setup
```

Each file is named after the app that contributed it.

## How to Write One

```go
// hyrums/travel_calendar_test.go
package hyrums

import (
    "os"
    "testing"

    "github.com/michaelwinser/appbase/db"
)

// travel-calendar depends on Migrate being idempotent.
// The app calls Migrate on every startup, and the schema uses
// CREATE TABLE IF NOT EXISTS. If Migrate errors on repeated calls,
// the app won't start.
func TestMigrateIsIdempotent(t *testing.T) {
    os.Setenv("STORE_TYPE", "sqlite")
    os.Setenv("SQLITE_DB_PATH", ":memory:")
    defer os.Unsetenv("SQLITE_DB_PATH")

    d, err := db.New()
    if err != nil {
        t.Fatal(err)
    }
    defer d.Close()

    schema := `CREATE TABLE IF NOT EXISTS things (id TEXT PRIMARY KEY);`
    if err := d.Migrate(schema); err != nil {
        t.Fatal(err)
    }
    // Second call must not error
    if err := d.Migrate(schema); err != nil {
        t.Fatal("Migrate is not idempotent:", err)
    }
}

// travel-calendar depends on session cookies being named "app_session".
// The frontend hardcodes this name. If it changes, login breaks.
func TestSessionCookieName(t *testing.T) {
    if auth.CookieName != "app_session" {
        t.Fatalf("expected cookie name 'app_session', got %q", auth.CookieName)
    }
}
```

## Key Principles

1. **Name the app** — the test file says who depends on this behavior
2. **Document the assumption** — the comment explains WHY the app cares
3. **Test the observable behavior** — not internal implementation
4. **Keep tests fast** — use in-memory SQLite, no network calls
5. **One assumption per test** — each test validates exactly one contract

## Process

### Adding from an app:

1. Identify the behavior you depend on
2. Write the test in `hyrums/yourapp_test.go`
3. Submit a PR to appbase
4. The test runs in appbase CI from now on

### When appbase changes break a Hyrum's test:

The appbase maintainer has three options:
1. **Preserve the behavior** — the dependent app is right, keep it working
2. **Update the test** — the behavior change is intentional, update the test and notify the app
3. **Remove the test** — the assumption was incorrect, remove it and fix the app

Option 2 is most common. The key: the app gets notified, not surprised.

## Running

```bash
# Run just Hyrum's tests
go test ./hyrums/... -v

# Run as part of full suite
go test ./...
```

These run in CI on every appbase push and PR.
