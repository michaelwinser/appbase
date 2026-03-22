package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func lintAPICmd() *cobra.Command {
	return &cobra.Command{
		Use:   "lint-api",
		Short: "Verify OpenAPI codegen is up to date",
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := os.Stat("openapi.yaml"); err != nil {
				fmt.Println("SKIP: No openapi.yaml found (not an API-first app)")
				return nil
			}

			errors := 0

			// Check 1: Generated code is up to date
			if isDir("api") {
				oapiCodegen, err := findOapiCodegen()
				if err != nil {
					fmt.Println("SKIP: oapi-codegen not installed (cannot verify generated code)")
				} else {
					tmpdir, _ := os.MkdirTemp("", "ab-lint-api-*")
					defer os.RemoveAll(tmpdir)

					if _, err := os.Stat("oapi-codegen.yaml"); err == nil {
						errors += checkGenerated(oapiCodegen, "oapi-codegen.yaml", "api/server.gen.go", tmpdir)
					}
					if _, err := os.Stat("oapi-codegen-client.yaml"); err == nil {
						errors += checkGenerated(oapiCodegen, "oapi-codegen-client.yaml", "api/client.gen.go", tmpdir)
					}
				}
			}

			// Check 2: No hand-written /api/ routes
			handwritten := findHandwrittenRoutes()
			if len(handwritten) > 0 {
				fmt.Println("WARN: Hand-written /api/ routes found (should be in openapi.yaml):")
				for _, line := range handwritten {
					fmt.Printf("  %s\n", line)
				}
			} else {
				fmt.Println("OK: No hand-written /api/ routes")
			}

			// Check 3: ServerInterface implementation
			if isDir("api") {
				if hasServerInterfaceImpl() {
					fmt.Println("OK: ServerInterface implementation found")
				} else {
					fmt.Println("FAIL: No ServerInterface implementation found")
					errors++
				}
			}

			if errors > 0 {
				return fmt.Errorf("lint-api: %d error(s) found", errors)
			}
			fmt.Println("API lint passed.")
			return nil
		},
	}
}

// checkGenerated re-runs codegen to a temp dir and diffs against the existing file.
func checkGenerated(oapiCodegen, configFile, genFile, tmpdir string) int {
	base := filepath.Base(genFile)
	tmpOutput := filepath.Join(tmpdir, base)
	tmpConfig := filepath.Join(tmpdir, "config-"+base+".yaml")

	// Create temp config with redirected output path
	data, err := os.ReadFile(configFile)
	if err != nil {
		return 0
	}
	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "output:") {
			lines = append(lines, fmt.Sprintf("output: %s", tmpOutput))
		} else {
			lines = append(lines, line)
		}
	}
	os.WriteFile(tmpConfig, []byte(strings.Join(lines, "\n")), 0644)

	// Run codegen
	cmd := exec.Command(oapiCodegen, "--config", tmpConfig, "openapi.yaml")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Printf("WARN: codegen failed for %s: %v\n", genFile, err)
		return 0
	}

	// Compare
	existing, _ := os.ReadFile(genFile)
	generated, _ := os.ReadFile(tmpOutput)
	if string(existing) != string(generated) {
		fmt.Printf("FAIL: %s is out of date. Run: ab codegen\n", genFile)
		return 1
	}
	fmt.Printf("OK: %s is up to date\n", genFile)
	return 0
}

// findHandwrittenRoutes searches Go files for /api/ route registrations
// outside generated code and test files.
func findHandwrittenRoutes() []string {
	var results []string
	filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, ".gen.go") {
			return nil
		}
		if filepath.Base(path) == "app.go" {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := scanner.Text()
			if (strings.Contains(line, `.Get("/api/`) ||
				strings.Contains(line, `.Post("/api/`) ||
				strings.Contains(line, `.Put("/api/`) ||
				strings.Contains(line, `.Delete("/api/`) ||
				strings.Contains(line, `.Patch("/api/`)) &&
				!strings.Contains(line, `/api/auth/`) {
				results = append(results, fmt.Sprintf("%s:%d: %s", path, lineNum, strings.TrimSpace(line)))
			}
		}
		return nil
	})
	return results
}

// hasServerInterfaceImpl checks if any non-generated Go file references ServerInterface.
func hasServerInterfaceImpl() bool {
	found := false
	filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || found {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, ".gen.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if strings.Contains(string(data), "ServerInterface") {
			found = true
		}
		return nil
	})
	return found
}
