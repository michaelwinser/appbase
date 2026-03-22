# Critical Review: appbase (March 2026)

## Overall Assessment

This is a well-structured, opinionated Go framework for building small-scale web apps with consistent infrastructure. The code is clean, readable, and demonstrates good separation of concerns. That said, there are meaningful issues across security, correctness, architecture, and testing.

## What's Working Well

- Consistent naming conventions and package organization
- Clean interface abstractions (sessionBackend, store backend)
- Immutable query builder pattern in `store`
- Good use of Go generics in `Collection[T]`
- Proper error wrapping throughout
- Empty slice (not nil) returned from queries
- Consumer contract tests (hyrums/) — smart idea
- Comprehensive CRUD + query tests in `store/store_test.go`
- Use case test harness pattern

---

## Issues Filed

### High Priority — Security

| # | Issue | Labels |
|---|-------|--------|
| [#3](https://github.com/michaelwinser/appbase/issues/3) | SQL injection in store Where clauses and table names | security, bug |
| [#4](https://github.com/michaelwinser/appbase/issues/4) | OAuth state parameter not validated (CSRF) | security, bug |
| [#5](https://github.com/michaelwinser/appbase/issues/5) | CORS allows all origins on authenticated endpoints | security, enhancement |
| [#6](https://github.com/michaelwinser/appbase/issues/6) | DevAuth creates a new session on every request | security, bug |
| [#14](https://github.com/michaelwinser/appbase/issues/14) | CLI cookie jar leaks session to any redirect target | bug |

### Medium Priority — Correctness

| # | Issue | Labels |
|---|-------|--------|
| [#7](https://github.com/michaelwinser/appbase/issues/7) | Silent time.Parse failures cause instant session expiry | bug |
| [#8](https://github.com/michaelwinser/appbase/issues/8) | Firestore in-memory filtering missing inequality operators | bug |
| [#9](https://github.com/michaelwinser/appbase/issues/9) | db.QueryRow returns nil for Firestore, causing nil dereference | bug |
| [#10](https://github.com/michaelwinser/appbase/issues/10) | store reflect.go silently drops unsupported field types | bug |

### Medium Priority — Architecture & Testing

| # | Issue | Labels |
|---|-------|--------|
| [#11](https://github.com/michaelwinser/appbase/issues/11) | Replace os.Setenv config propagation with explicit config passing | enhancement |
| [#13](https://github.com/michaelwinser/appbase/issues/13) | App.Router() return type hides chi.Router capabilities | enhancement |
| [#15](https://github.com/michaelwinser/appbase/issues/15) | Major test coverage gaps: auth middleware, google OAuth, CLI, app.go | testing |

### Low Priority

| # | Issue | Labels |
|---|-------|--------|
| [#12](https://github.com/michaelwinser/appbase/issues/12) | Add graceful shutdown and context propagation | enhancement |
| [#16](https://github.com/michaelwinser/appbase/issues/16) | Reduce transitive dependency weight (Wails, CGO) | enhancement |
| [#17](https://github.com/michaelwinser/appbase/issues/17) | Minor correctness issues: RespondJSON, .env parser, Config.Set, preflight table | bug |

---

## Notes

The foundation is solid — clean Go, good abstractions, thoughtful layering. The issues above are largely about hardening for production use. The biggest risks are the SQL injection surface (#3) and OAuth CSRF (#4), both of which are straightforward to fix.

Issue #11 (config propagation) is the largest architectural change and would touch most packages. Worth planning carefully — the global mutable state in `cli` (package-level vars `IsServeCommand`, `IsLocalMode`, `AutoServeHandler`) is part of the same concern.

Issue #17 bundles five smaller items that can be addressed individually or together.
