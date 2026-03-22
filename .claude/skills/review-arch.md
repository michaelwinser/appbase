---
name: review-arch
description: Review staged or recent changes for architectural conformance
trigger: When the user asks to review architecture, check invariants, or before committing architectural changes
---

# Architectural Review

Review changes against the architectural invariants documented in CLAUDE.md and `docs/architecture-local-mode.md`.

## Process

1. **Get the diff** — staged changes, or if nothing staged, changes since last commit:

```bash
# Prefer staged changes; fall back to last commit
git diff --cached --stat || git diff HEAD~1 --stat
```

```bash
git diff --cached || git diff HEAD~1
```

2. **Read the invariants** — load both sources of architectural truth:
   - `CLAUDE.md` (the "Architectural Invariants" section)
   - `docs/architecture-local-mode.md`

3. **Check each invariant against the diff.** For each changed file, ask:

### Identity injection

- Does any change add middleware that sets user identity (context values for userID/email)?
- Does any change create sessions or set cookies outside of `auth/google.go` or `auth/middleware.go`?
- Does any change add `AUTH_MODE` checks or `DevAuth` references?

**Correct pattern:** Identity enters via `handlerTransport` (CLI), `LocalHandler()` (desktop), session cookie (web). Nothing else.

### Config propagation

- Does any change call `os.Setenv()` in library code (not tests, not CLI `main()`)?
- Does any change read config via `os.Getenv()` where a struct field or parameter could be used instead?

**Correct pattern:** Config passed explicitly via structs/parameters. Env vars read at edges only.

### Handler mode-awareness

- Does any change add mode-specific logic (`if localMode`, `if isDesktop`) inside an HTTP handler?
- Does any change add endpoint overrides (re-registering a route to change behavior per mode)?

**Correct pattern:** Handlers are mode-agnostic. Mode differences handled at the transport/wrapper layer.

### Shortcut detection

- Does any change work around an existing pattern rather than extending it?
- Does any change duplicate logic that already exists in a different form?
- Does any change add a "temporary" fix with a TODO?

**If a shortcut is detected:** Explain what the shortcut is, what pattern it circumvents, and what the pattern-conforming solution would be. Flag it clearly.

## Output Format

For each finding, report:
- **File and line range**
- **Which invariant** is affected
- **What the change does** (brief)
- **Why it's a concern**
- **What to do instead**

If no violations found, confirm: "No architectural violations detected in [N files changed]."

## Severity Levels

- **VIOLATION** — directly breaks an invariant. Must fix before committing.
- **WARNING** — doesn't break an invariant but moves toward a known anti-pattern. Discuss with user.
- **NOTE** — worth mentioning but not blocking. Informational.
