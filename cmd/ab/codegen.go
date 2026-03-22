package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

func codegenCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "codegen [openapi.yaml]",
		Short: "Generate server + client from OpenAPI spec",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			spec := "openapi.yaml"
			if len(args) > 0 {
				spec = args[0]
			}
			if _, err := os.Stat(spec); err != nil {
				return fmt.Errorf("no %s found — create an OpenAPI spec first", spec)
			}

			oapiCodegen, err := findOapiCodegen()
			if err != nil {
				return err
			}

			os.MkdirAll("api", 0755)

			// Generate server
			if _, err := os.Stat("oapi-codegen.yaml"); err == nil {
				fmt.Println("Generating server from", spec, "...")
				if err := run(oapiCodegen, "--config", "oapi-codegen.yaml", spec); err != nil {
					return fmt.Errorf("server codegen failed: %w", err)
				}
			} else {
				fmt.Println("Generating server from", spec, "(default config)...")
				if err := run(oapiCodegen, "-generate", "chi-server,types", "-package", "api", "-o", "api/server.gen.go", spec); err != nil {
					return fmt.Errorf("server codegen failed: %w", err)
				}
			}

			// Generate client
			if _, err := os.Stat("oapi-codegen-client.yaml"); err == nil {
				fmt.Println("Generating client from", spec, "...")
				if err := run(oapiCodegen, "--config", "oapi-codegen-client.yaml", spec); err != nil {
					return fmt.Errorf("client codegen failed: %w", err)
				}
			} else {
				fmt.Println("Generating client from", spec, "(default config)...")
				if err := run(oapiCodegen, "-generate", "client", "-package", "api", "-o", "api/client.gen.go", spec); err != nil {
					return fmt.Errorf("client codegen failed: %w", err)
				}
			}

			// TypeScript types if frontend exists
			if isDir("frontend") || isDir("src") {
				if _, err := exec.LookPath("npx"); err == nil {
					outdir := "frontend/src/lib"
					if isDir("src") && !isDir("frontend") {
						outdir = "src/lib"
					}
					os.MkdirAll(outdir, 0755)
					fmt.Println("Generating TypeScript types...")
					run("npx", "openapi-typescript", spec, "-o", outdir+"/api.d.ts")
				}
			}

			fmt.Println("Codegen complete.")
			return nil
		},
	}
}

func findOapiCodegen() (string, error) {
	// Check PATH
	if path, err := exec.LookPath("oapi-codegen"); err == nil {
		return path, nil
	}
	// Check GOPATH/bin
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		out, err := exec.Command("go", "env", "GOPATH").Output()
		if err == nil {
			gopath = string(out[:len(out)-1]) // trim newline
		}
	}
	if gopath != "" {
		path := gopath + "/bin/oapi-codegen"
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	// Try to install
	fmt.Println("Installing oapi-codegen...")
	if err := run("go", "install", "github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest"); err != nil {
		return "", fmt.Errorf("oapi-codegen not found and install failed: %w", err)
	}
	if gopath != "" {
		return gopath + "/bin/oapi-codegen", nil
	}
	return "", fmt.Errorf("oapi-codegen installed but GOPATH unknown")
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func isDir(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}
