// Command appbase is the appbase CLI tool.
//
// Install:
//
//	go install github.com/michaelwinser/appbase/cmd/appbase@latest
//
// Usage from any project directory:
//
//	appbase init                          # create app.yaml
//	appbase secret set <name> <value>     # store secret in OS keychain
//	appbase secret import <creds.json>    # import Google OAuth credentials
//	appbase codegen                       # generate server + client from OpenAPI spec
//	appbase lint-api                      # verify codegen is up to date
//	appbase deploy                        # deploy to Cloud Run
//	appbase provision <email>             # full GCP setup
//	appbase docker up|down|logs           # local Docker
//
// Reads app.yaml and app.json from the current working directory.
package main

import (
	"fmt"
	"os"
	"runtime/debug"

	"github.com/michaelwinser/appbase/deploy"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "appbase",
	Short: "appbase CLI — project management for appbase apps",
}

// applyDBFlag sets SQLITE_DB_PATH from the --db flag if provided.
func applyDBFlag(cmd *cobra.Command) {
	dbPath, _ := cmd.Flags().GetString("db")
	if dbPath != "" {
		os.Setenv("SQLITE_DB_PATH", dbPath)
	}
}

func main() {
	rootCmd.AddCommand(secretCmd())
	rootCmd.AddCommand(codegenCmd())
	rootCmd.AddCommand(lintAPICmd())
	rootCmd.AddCommand(initCmd())
	rootCmd.AddCommand(deployCmd())
	rootCmd.AddCommand(provisionCmd())
	rootCmd.AddCommand(dockerCmd())
	rootCmd.AddCommand(testSessionCmd())
	rootCmd.AddCommand(testLoginCmd())
	rootCmd.AddCommand(testLogoutCmd())
	rootCmd.AddCommand(targetCmd())
	rootCmd.AddCommand(updateCmd())
	rootCmd.AddCommand(devTemplateCmd())
	rootCmd.AddCommand(sandboxTemplateCmd())
	rootCmd.AddCommand(versionCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func devTemplateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "dev-template",
		Short: "Print dev-template.sh to stdout (for eval in ./dev scripts)",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Print(deploy.DevTemplate)
		},
	}
}

func sandboxTemplateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sandbox-template",
		Short: "Print sandbox script template to stdout (for nono integration)",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Print(deploy.SandboxTemplate)
		},
	}
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print appbase version",
		Run: func(cmd *cobra.Command, args []string) {
			version := "dev"
			if info, ok := debug.ReadBuildInfo(); ok {
				version = info.Main.Version
			}
			fmt.Printf("appbase CLI %s\n", version)
		},
	}
}
