---
name: add-capability
description: Add a new capability to appbase (e.g., new store backend, auth provider, middleware)
trigger: When the user asks to add a new feature or capability to the appbase module
---

# Adding a Capability to appbase

This is a shared Go module. Changes affect all dependent apps. Follow this checklist.

## Before You Start

1. Read `CLAUDE.md` for architecture overview
2. Read `TESTING.md` for testing conventions
3. Check if the capability already exists or partially exists
4. Consider backward compatibility — can you add without breaking existing callers?

## Steps

### 1. Design the API

Define the exported types and functions first. Write the Go interface or function signatures before implementing. Ask yourself:
- Does this belong in an existing package (db, auth, server, config, cli) or a new one?
- What's the minimal API surface? Start small.
- Can apps opt in without affecting apps that don't use it?

### 2. Implement

- Put code in the appropriate package
- Follow existing patterns (see similar code in the package)
- Use the `config` package for any new configuration
- Environment variables follow the convention: `dot.notation` → `DOT_NOTATION`
- If adding a new store backend: implement `db/preflight.go` support so the startup check covers it
- If adding anything that touches both SQL and Firestore: follow the backend interface pattern (see `auth/session.go` and `examples/todo/store.go`)

### 3. Add Tests

- Unit tests in `*_test.go` alongside the code
- If the capability is user-facing, add use case tests to the todo example

### 4. Update the Todo Example

The todo app in `examples/todo/` must exercise all appbase capabilities. If you added something new, show it in use.

### 5. Update Documentation

- Update `CLAUDE.md` package structure diagram if you added a new package
- Update `CLAUDE.md` environment variables table if you added config
- Update `TESTING.md` if the testing approach changed

### 6. Verify

```bash
go build ./...
go test ./...
```

All tests must pass, including the todo example's use case tests.

### 7. Consider Consumer Impact

- Will this change break any dependent apps?
- Should dependent apps add a Hyrum's test for this behavior?
- If it's a breaking change, document the migration path
