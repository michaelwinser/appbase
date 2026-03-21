# Consumer Contract Tests (Hyrum's Tests)

This directory contains tests contributed by applications that depend on appbase. Each test validates an assumption the app makes about appbase's behavior.

## Why

When appbase changes, these tests catch unintended breakage in dependent apps before it ships. They document the actual usage patterns — not just what appbase *intends* to support, but what apps *actually rely on*.

See [github.com/michaelwinser/hyrums-tests](https://github.com/michaelwinser/hyrums-tests) for the philosophy.

## How to Contribute

1. Create a test file named after your app: `travel_calendar_test.go`
2. Write tests that validate your assumptions about appbase behavior
3. Submit a PR to appbase with your tests

### Example

```go
package hyrums

import (
    "testing"
    "github.com/michaelwinser/appbase/db"
)

// travel-calendar assumes that DB.Migrate can be called multiple times
// safely (CREATE TABLE IF NOT EXISTS pattern).
func TestMigrateIsIdempotent(t *testing.T) {
    d := setupTestDB(t)
    schema := `CREATE TABLE IF NOT EXISTS test (id TEXT PRIMARY KEY);`

    // First call
    if err := d.Migrate(schema); err != nil {
        t.Fatal(err)
    }
    // Second call should not error
    if err := d.Migrate(schema); err != nil {
        t.Fatal("Migrate is not idempotent:", err)
    }
}
```

## Running

```bash
go test ./hyrums/...
```

These tests run as part of `go test ./...` — they must pass before any appbase release.
