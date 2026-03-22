// Command ab is the appbase CLI tool.
//
// Install:
//
//	go install github.com/michaelwinser/appbase/cmd/ab@latest
//
// Usage from any project directory:
//
//	ab init                          # create app.yaml
//	ab secret set <name> <value>     # store secret in OS keychain
//	ab secret import <creds.json>    # import Google OAuth credentials
//	ab codegen                       # generate server + client from OpenAPI spec
//	ab lint-api                      # verify codegen is up to date
//	ab deploy                        # deploy to Cloud Run
//	ab provision <email>             # full GCP setup
//	ab docker up|down|logs           # local Docker
//
// Reads app.yaml and app.json from the current working directory.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "ab",
	Short: "appbase CLI — project management for appbase apps",
}

func main() {
	rootCmd.AddCommand(secretCmd())
	rootCmd.AddCommand(codegenCmd())
	rootCmd.AddCommand(lintAPICmd())
	rootCmd.AddCommand(initCmd())
	rootCmd.AddCommand(deployCmd())
	rootCmd.AddCommand(provisionCmd())
	rootCmd.AddCommand(dockerCmd())
	rootCmd.AddCommand(versionCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print ab version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("ab (appbase CLI) v0.1.0")
		},
	}
}
