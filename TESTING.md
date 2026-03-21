# Testing Strategy

## Layers

| Layer | What | How | Where |
|-------|------|-----|-------|
| **Unit** | Individual functions, store methods | `go test` | `*_test.go` alongside code |
| **Use Case (E2E)** | Numbered use cases from PRD | HTTP harness via `testing.Harness` | `usecases_test.go` in app |
| **Browser** | UI interactions, visual | Playwright | `tests/browser/` in app |
| **Consumer Contract** | Appbase behavior assumptions | `go test` in appbase | `hyrums/` in appbase |

## Use Case Tests

Every use case in the PRD gets a numbered test: `UC-XXXX`. Tests are the executable form of acceptance criteria.

### Writing Use Case Tests

```go
func TestUseCases(t *testing.T) {
    h := harness.New(t, setupApp)

    h.Run("UC-1001", "Create a multi-day activity", func(c *harness.Client) {
        resp := c.POST("/api/activities", `{
            "title": "London Trip",
            "startDate": "2026-04-01",
            "endDate": "2026-04-05",
            "location": "London"
        }`)
        c.AssertStatus(resp, 201)
        c.AssertJSONHas(resp, "title", "London Trip")
    })

    h.Run("UC-1002", "Detect location conflict", func(c *harness.Client) {
        // Create an activity away from home
        c.POST("/api/activities", `{...away activity...}`)
        // Create a local commitment on the same day
        c.POST("/api/activities", `{...local commitment...}`)
        // Check conflicts
        resp := c.GET("/api/conflicts")
        c.AssertStatus(resp, 200)
        c.AssertJSONArray(resp, 1) // One conflict
    })
}
```

### Naming Convention

- Test function: `TestUseCases`
- Subtest: `UC-XXXX_Description_with_underscores`
- PRD reference: Each UC-XXXX maps to a use case in the PRD or TESTING.md

### Where to Document Use Cases

Option A: In the PRD (`docs/prd/*.md`) with acceptance criteria
Option B: In a dedicated `TESTING.md` at the app root

Either way, the test code references the UC-XXXX ID so you can trace from test failure → requirement.

## CLI Coverage

The CLI exercises the same API as the web UI. Use case tests run against the HTTP API, which means they validate both the CLI path and the web path.

```
CLI → API → Service → Store → DB
Web → API → Service → Store → DB
Test → API → Service → Store → DB  ← same path
```

## Running Tests

### From the CLI

```bash
# All tests
myapp test

# Specific package
myapp test ./internal/store/...
```

### From the project script

```bash
# All tests suitable for CI
./tc test

# Just backend
./tc test backend

# Just browser
./tc test browser
```

### In CI (GitHub Actions)

The CI workflow runs:
1. `go build ./...` — compilation check
2. `go test ./...` — all Go tests (unit + use case)
3. Playwright tests (if configured)

## The Harness

`github.com/michaelwinser/appbase/testing` provides:

- **`Harness`** — spins up a real HTTP server with in-memory DB
- **`Client`** — HTTP client with cookie tracking (for session tests)
- **`Response`** — response wrapper with JSON parsing helpers
- **Assertions** — `AssertStatus`, `AssertJSONHas`, `AssertJSONArray`

The harness uses `httptest.Server` so tests run fast (no port binding, no Docker).

## Consumer Contract Tests (Hyrum's)

Apps that depend on appbase contribute tests to `hyrums/` that document their actual usage. When appbase changes, these tests catch unintended breakage.

See `hyrums/README.md` for details.
