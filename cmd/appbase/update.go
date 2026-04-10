package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/michaelwinser/appbase/deploy"
	"github.com/spf13/cobra"
)

func updateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update project to latest appbase version and patterns",
		RunE: func(cmd *cobra.Command, args []string) error {
			dryRun, _ := cmd.Flags().GetBool("dry-run")

			if _, err := os.Stat("go.mod"); err != nil {
				return fmt.Errorf("no go.mod found — run from a Go project directory")
			}

			if dryRun {
				fmt.Println("Dry run — no changes will be made.")
				fmt.Println()
			}

			updated := 0
			suggestions := 0

			// 1. Bump appbase dependency
			if result := updateGoMod(dryRun); result != "" {
				fmt.Println(result)
				updated++
			}

			// 2. Check ./dev script
			if result := checkDevScript(dryRun); result != "" {
				fmt.Println(result)
				suggestions++
			}

			// 3. Check for .mise.toml
			if result := checkMiseToml(dryRun); result != "" {
				fmt.Println(result)
				suggestions++
			}

			// 4. Update ./sandbox script (overwrite boilerplate, preserve sandbox.flags)
			if result := updateSandbox(dryRun); result != "" {
				fmt.Println(result)
				updated++
			}

			// 5. Update Claude Code skills
			if result := updateSkills(dryRun); result != "" {
				fmt.Println(result)
				updated++
			}

			// 6. Detect outdated code patterns
			patterns := detectOutdatedPatterns()
			for _, p := range patterns {
				fmt.Printf("  suggest: %s\n", p)
				suggestions++
			}

			fmt.Println()
			if updated == 0 && suggestions == 0 {
				fmt.Println("Everything is up to date.")
			} else {
				if updated > 0 {
					fmt.Printf("%d updated.", updated)
				}
				if suggestions > 0 {
					if updated > 0 {
						fmt.Print(" ")
					}
					fmt.Printf("%d suggestions — review above.", suggestions)
				}
				fmt.Println()
			}
			return nil
		},
	}
	cmd.Flags().Bool("dry-run", false, "Show what would change without making changes")
	return cmd
}

func updateGoMod(dryRun bool) string {
	// Check if appbase is a dependency
	data, err := os.ReadFile("go.mod")
	if err != nil {
		return ""
	}
	if !strings.Contains(string(data), "michaelwinser/appbase") {
		return ""
	}

	// Find current version
	current := ""
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.Contains(line, "michaelwinser/appbase") && !strings.Contains(line, "module") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				current = parts[len(parts)-1]
			}
		}
	}

	if dryRun {
		return fmt.Sprintf("  would update: go get github.com/michaelwinser/appbase@latest (current: %s)", current)
	}

	fmt.Printf("  updating: go get github.com/michaelwinser/appbase@latest (current: %s)...\n", current)
	cmd := exec.Command("go", "get", "github.com/michaelwinser/appbase@latest")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Sprintf("  error: go get failed: %v", err)
	}

	// Run go mod tidy
	tidy := exec.Command("go", "mod", "tidy")
	tidy.Stdout = os.Stdout
	tidy.Stderr = os.Stderr
	tidy.Run()

	return "  updated: appbase dependency bumped to latest"
}

func checkDevScript(dryRun bool) string {
	data, err := os.ReadFile("dev")
	if err != nil {
		return ""
	}
	content := string(data)

	// Check if still using sibling source path
	if strings.Contains(content, ". \"../") || strings.Contains(content, ". '../") {
		return "  suggest: ./dev still sources from sibling path.\n" +
			"           Replace with: eval \"$(appbase dev-template)\""
	}

	// Check if using eval pattern (already current)
	if strings.Contains(content, "appbase dev-template") {
		// ./dev is self-updating via eval — the installed binary is the update
		return ""
	}

	return ""
}

func checkMiseToml(dryRun bool) string {
	if _, err := os.Stat(".mise.toml"); err == nil {
		return "" // already exists
	}
	return "  suggest: no .mise.toml found.\n" +
		"           Create one for toolchain management: mise use go node \"npm:pnpm\""
}

// updateSandbox manages ./sandbox as a fully-overwritten file.
// Project-specific flags live in ./sandbox.flags, which is never touched
// once it exists. Legacy scripts with inlined flags are migrated.
func updateSandbox(dryRun bool) string {
	existing, err := os.ReadFile("sandbox")
	if err != nil {
		// No sandbox script — leave as suggestion (don't force nono on everyone)
		fmt.Println("  suggest: no ./sandbox script found.\n" +
			"           Create one: appbase sandbox-template > sandbox && chmod +x sandbox")
		return ""
	}

	if string(existing) == deploy.SandboxTemplate {
		return "" // already current
	}

	// Migrate inlined flags to sandbox.flags if needed
	migrateMsg := ""
	if _, err := os.Stat("sandbox.flags"); err != nil {
		flags := extractSandboxFlags(string(existing))
		content := deploy.SandboxFlagsTemplate
		if len(flags) > 0 {
			content = "# Project-specific nono sandbox capabilities.\n" +
				"# Sourced by ./sandbox — edit this file, not ./sandbox.\n\n" +
				strings.Join(flags, "\n") + "\n"
			migrateMsg = fmt.Sprintf(" (migrated %d flags to sandbox.flags)", len(flags))
		}
		if dryRun {
			fmt.Printf("  would create: sandbox.flags%s\n", migrateMsg)
		} else {
			if err := os.WriteFile("sandbox.flags", []byte(content), 0644); err != nil {
				return fmt.Sprintf("  error: writing sandbox.flags: %v", err)
			}
		}
	}

	if dryRun {
		return "  would update: ./sandbox (regenerate from template)"
	}
	if err := os.WriteFile("sandbox", []byte(deploy.SandboxTemplate), 0755); err != nil {
		return fmt.Sprintf("  error: writing sandbox: %v", err)
	}
	return "  updated: ./sandbox regenerated from template" + migrateMsg
}

// extractSandboxFlags pulls project-specific nono flags from a legacy
// sandbox script for migration to sandbox.flags. It captures both
// inlined flags in the exec line and APP_FLAGS= assignments, skipping
// the standard $PROJECT_DIR allow that the template already provides.
func extractSandboxFlags(script string) []string {
	var out []string
	seen := make(map[string]bool)

	// nono flags with their argument: --allow PATH, --read PATH, --allow-bind PORT
	flagRe := regexp.MustCompile(`--(?:allow|read|allow-bind)\s+("[^"]*"|\$[A-Za-z_][A-Za-z0-9_]*|[^\s\\]+)`)

	for _, line := range strings.Split(script, "\n") {
		trimmed := strings.TrimSpace(line)

		// Skip comments
		if strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Already-correct APP_FLAGS lines: keep verbatim
		if strings.HasPrefix(trimmed, "APP_FLAGS=") && trimmed != `APP_FLAGS=""` {
			if !seen[trimmed] {
				out = append(out, trimmed)
				seen[trimmed] = true
			}
			continue
		}

		// Inlined flags (e.g. in the `exec nono run \` block)
		for _, m := range flagRe.FindAllString(line, -1) {
			// Skip standard flags the template already provides
			if strings.Contains(m, "$PROJECT_DIR") || strings.Contains(m, "$GO_BIN_DIR") {
				continue
			}
			emit := fmt.Sprintf(`APP_FLAGS="$APP_FLAGS %s"`, m)
			if !seen[emit] {
				out = append(out, emit)
				seen[emit] = true
			}
		}
	}
	return out
}

func updateSkills(dryRun bool) string {
	skills := deploy.ConsumerSkills()
	if len(skills) == 0 {
		return ""
	}

	skillsDir := filepath.Join(".claude", "skills")
	wrote := 0
	skipped := 0

	for name, content := range skills {
		path := filepath.Join(skillsDir, name)

		// Check if already up to date
		existing, err := os.ReadFile(path)
		if err == nil && string(existing) == content {
			skipped++
			continue
		}

		if dryRun {
			if err != nil {
				fmt.Printf("  would add: %s\n", path)
			} else {
				fmt.Printf("  would update: %s\n", path)
			}
			wrote++
			continue
		}

		os.MkdirAll(skillsDir, 0755)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			fmt.Printf("  error writing %s: %v\n", path, err)
			continue
		}
		wrote++
	}

	if wrote == 0 {
		return ""
	}
	if dryRun {
		return fmt.Sprintf("  would update %d Claude Code skills in %s", wrote, skillsDir)
	}
	return fmt.Sprintf("  updated: %d Claude Code skills in %s", wrote, skillsDir)
}

func detectOutdatedPatterns() []string {
	var issues []string

	// Check Go source files for outdated patterns
	goFiles := findGoFiles(".")
	for _, path := range goFiles {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		content := string(data)

		if strings.Contains(content, "ResolveServerWithAutoServe") {
			issues = append(issues, fmt.Sprintf(
				"%s uses deprecated ResolveServerWithAutoServe — use appcli.ClientForCommand instead", path))
		}

		if strings.Contains(content, "AUTH_MODE") && strings.Contains(content, "dev") {
			issues = append(issues, fmt.Sprintf(
				"%s references AUTH_MODE=dev (deprecated) — use Config{LocalMode: true} instead", path))
		}

		if strings.Contains(content, "testSessions.Create") || strings.Contains(content, "sessions.Create") {
			if strings.HasSuffix(path, "_test.go") {
				issues = append(issues, fmt.Sprintf(
					"%s creates sessions for testing — use APPBASE_TEST_MODE=true + X-Test-User header instead", path))
			}
		}
	}

	// Check CLAUDE.md for devcontainer-only rules
	if data, err := os.ReadFile("CLAUDE.md"); err == nil {
		content := string(data)
		if strings.Contains(content, "Do not install Node") && !strings.Contains(content, "mise") {
			issues = append(issues, "CLAUDE.md has devcontainer-only frontend rules — consider adding mise as an alternative")
		}
	}

	return issues
}

// findGoFiles returns .go files in the directory tree, skipping vendor and hidden dirs.
func findGoFiles(root string) []string {
	var files []string
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") || name == "vendor" || name == "api" {
			continue
		}
		path := root + "/" + name
		if e.IsDir() {
			files = append(files, findGoFiles(path)...)
		} else if strings.HasSuffix(name, ".go") {
			files = append(files, path)
		}
	}
	return files
}
