# Claude Code Settings for appbase Projects

## Recommended Permissions

Apps built on appbase use Go, git, gcloud, Docker, and the `appbase` CLI frequently. Without permission rules, Claude Code prompts for each command individually.

Create `.claude/settings.local.json` in your project root (gitignored — personal to your workstation):

```json
{
  "permissions": {
    "allow": [
      "Bash(go:*)",
      "Bash(git:*)",
      "Bash(sh:*)",
      "Bash(appbase:*)",
      "Bash(gcloud:*)",
      "Bash(docker:*)",
      "Bash(gh:*)",
      "Bash(mkdir:*)",
      "Bash(ls:*)",
      "Bash(rm -f:*)",
      "Bash(export:*)",
      "Bash(security:*)",
      "Bash(curl:*)",
      "Bash(oapi-codegen:*)",
      "Bash(head:*)",
      "Bash(cat:*)",
      "Bash(cd /Users/YOURUSERNAME/path/to/projects:*)"
    ]
  }
}
```

Adjust the `cd` path to match your projects directory.

## What each rule covers

| Rule | Operations |
|------|-----------|
| `Bash(go:*)` | `go build`, `go test`, `go run`, `go mod tidy`, `go install`, `go vet`, `go get` |
| `Bash(git:*)` | `git status`, `git add`, `git commit`, `git push`, `git diff`, `git log`, `git tag` |
| `Bash(sh:*)` | Running shell scripts (`sh deploy_test.sh`, `sh -n script.sh`) |
| `Bash(appbase:*)` | `appbase secret`, `appbase codegen`, `appbase lint-api`, `appbase deploy`, etc. |
| `Bash(gcloud:*)` | GCP operations during provision and deploy |
| `Bash(docker:*)` | Docker compose operations |
| `Bash(gh:*)` | GitHub CLI (issues, PRs, releases) |
| `Bash(security:*)` | macOS Keychain operations (secret management) |
| `Bash(cd /path:*)` | Compound commands like `cd /path && git status` that agents use |

## Scoping

All Bash commands run in a sandbox scoped to the project directory. `rm -f` can only affect files within your project, not system files.

## Where to put settings

| File | Scope | Git | Use for |
|------|-------|-----|---------|
| `~/.claude/settings.json` | All projects | N/A | Global preferences |
| `.claude/settings.json` | This project, all users | Committed | Team-wide rules |
| `.claude/settings.local.json` | This project, just you | Gitignored | Personal permissions |

Permissions with filesystem paths (like the `cd` rule) should go in `.local.json` since paths differ per developer.

## Team-wide settings

If your team uses Claude Code, you can commit shared settings in `.claude/settings.json`:

```json
{
  "permissions": {
    "allow": [
      "Bash(go:*)",
      "Bash(git:*)",
      "Bash(sh:*)",
      "Bash(appbase:*)",
      "Bash(oapi-codegen:*)"
    ]
  }
}
```

Keep platform-specific rules (security, cd paths) in `.local.json`.

## The cd pattern

Claude Code agents sometimes run `cd /project/path && command` to ensure the correct working directory. This doesn't match a rule like `Bash(git:*)` because the command string starts with `cd`, not `git`. The `Bash(cd /path:*)` rule handles this.
