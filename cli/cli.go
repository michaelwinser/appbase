// Package cli provides a base CLI framework for appbase applications.
//
// It sets up a Cobra root command with common flags and built-in subcommands
// (serve, migrate). Applications add their own domain-specific commands.
//
// Usage:
//
//	app := appbase.New(config)
//	cli := appcli.New(app, "myapp", "My application description")
//	cli.AddCommand(myCustomCommand)
//	cli.Execute()
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// CLI wraps a Cobra root command with appbase integration.
type CLI struct {
	root    *cobra.Command
	setupFn func() error // called before commands that need the app
}

// New creates a new CLI with a root command.
// The setupFn is called before commands that need the app initialized
// (e.g., serve, migrate). It's NOT called for help or version.
func New(name, description string, setupFn func() error) *CLI {
	root := &cobra.Command{
		Use:   name,
		Short: description,
		// Don't print usage on errors from RunE
		SilenceUsage: true,
	}

	c := &CLI{root: root, setupFn: setupFn}
	c.addBuiltinCommands()
	return c
}

// AddCommand adds a command to the CLI.
func (c *CLI) AddCommand(cmd *cobra.Command) {
	c.root.AddCommand(cmd)
}

// Root returns the root cobra command for advanced configuration.
func (c *CLI) Root() *cobra.Command {
	return c.root
}

// Execute runs the CLI. Call this from main().
func (c *CLI) Execute() {
	if err := c.root.Execute(); err != nil {
		os.Exit(1)
	}
}

func (c *CLI) addBuiltinCommands() {
	// serve — start the HTTP server
	c.root.AddCommand(&cobra.Command{
		Use:   "serve",
		Short: "Start the HTTP server",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := c.setupFn(); err != nil {
				return err
			}
			// The serve implementation is provided by the app via setupFn
			// which initializes the app and starts serving.
			// This is a placeholder — the app overrides this.
			return fmt.Errorf("serve not implemented — override with cli.SetServeFunc()")
		},
	})

	// version — print version info
	c.root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("%s (built with appbase)\n", c.root.Use)
		},
	})
}

// SetServeFunc sets the function called by the "serve" command.
func (c *CLI) SetServeFunc(fn func() error) {
	for _, cmd := range c.root.Commands() {
		if cmd.Use == "serve" {
			cmd.RunE = func(cmd *cobra.Command, args []string) error {
				if err := c.setupFn(); err != nil {
					return err
				}
				return fn()
			}
			return
		}
	}
}

// Command creates a new command that runs setupFn before executing.
// Use this for commands that need the app initialized (database access, etc.).
func (c *CLI) Command(use, short string, runFn func(cmd *cobra.Command, args []string) error) *cobra.Command {
	return &cobra.Command{
		Use:          use,
		Short:        short,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := c.setupFn(); err != nil {
				return err
			}
			return runFn(cmd, args)
		},
	}
}
