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
	"net/http"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

// IsServeCommand is true when the "serve" command is being executed.
// Check this in your setupFn to set Config.Quiet for non-serve commands.
var IsServeCommand bool

// IsLocalMode is true when the CLI runs without --server (not the serve command).
// When true, the app should use LocalMode for single-user operation without OAuth.
// Check this in setupFn to set appbase.Config{LocalMode: true}.
var IsLocalMode bool

// LocalDataPath is the data directory for local mode (e.g., ~/.config/<appname>).
// Set automatically when IsLocalMode is true. Use this to set Config.DB.SQLitePath
// in your setupFn: config.DB.SQLitePath = appcli.LocalDataPath + "/app.db"
var LocalDataPath string

// cliName is the CLI binary name (from the root command's Use field).
// Used internally to ensure keychain lookups use the same name as login.
var cliName string

// Deprecated: Use ClientForCommand with an in-process handler transport instead.
// AutoServeHandler is set by the app after setup to enable auto-serve.
// When a CLI command runs without --server, the CLI starts an ephemeral
// server using this handler, runs the command, and tears down.
// Set this in your setupFn after initializing the app:
//
//	appcli.AutoServeHandler = app.Server().Router()
var AutoServeHandler http.Handler

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
		// Detect local mode for all commands (not just cli.Command()-created ones)
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			// Check command name since IsServeCommand hasn't been set yet
			// (RunE runs after PersistentPreRun).
			if cmd.Use == "serve" {
				IsServeCommand = true
			}
			if !IsServeCommand {
				localFlag, _ := cmd.Flags().GetBool("local")
				serverFlag, _ := cmd.Flags().GetString("server")
				if localFlag || serverFlag == "" {
					IsLocalMode = true

					// Determine data directory for local mode
					dataPath, _ := cmd.Flags().GetString("data")
					if dataPath == "" {
						home, _ := os.UserHomeDir()
						if home != "" {
							dataPath = home + "/.config/" + name
						}
					}
					if dataPath != "" {
						os.MkdirAll(dataPath, 0755)
						LocalDataPath = dataPath
					}
				}
			}
		},
	}

	cliName = name

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
			IsServeCommand = true
			if err := c.setupFn(); err != nil {
				return err
			}
			return fmt.Errorf("serve not implemented — override with cli.SetServeFunc()")
		},
	})

	// test — run all tests (for CI)
	testCmd := &cobra.Command{
		Use:   "test",
		Short: "Run all tests (unit, use case, integration)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTests(args)
		},
	}
	c.root.AddCommand(testCmd)

	// version — print version info
	c.root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("%s (built with appbase)\n", c.root.Use)
		},
	})

	// Persistent flags for CLI commands
	c.root.PersistentFlags().String("server", "", "Server URL (default: from keychain or http://localhost:3000)")
	c.root.PersistentFlags().Bool("local", false, "Force local/in-process mode (ignore saved server URL)")
	c.root.PersistentFlags().String("data", "", "Data directory (default: ~/.config/<appname>/ in local mode)")

	appName := c.root.Use

	// login — authenticate via browser
	c.root.AddCommand(&cobra.Command{
		Use:   "login",
		Short: "Log in via browser (Google OAuth)",
		RunE: func(cmd *cobra.Command, args []string) error {
			serverFlag, _ := cmd.Flags().GetString("server")
			serverURL := ResolveServerURL(serverFlag, appName)
			return LoginBrowser(serverURL, appName)
		},
	})

	// logout — clear session
	c.root.AddCommand(&cobra.Command{
		Use:   "logout",
		Short: "Log out and clear saved session",
		RunE: func(cmd *cobra.Command, args []string) error {
			return Logout(appName)
		},
	})

	// whoami — show current user
	c.root.AddCommand(&cobra.Command{
		Use:   "whoami",
		Short: "Show the current logged-in user",
		RunE: func(cmd *cobra.Command, args []string) error {
			serverFlag, _ := cmd.Flags().GetString("server")
			serverURL := ResolveServerURL(serverFlag, appName)
			return Whoami(serverURL, appName)
		},
	})
}

// runTests executes `go test ./...` or a specific package.
func runTests(args []string) error {
	target := "./..."
	if len(args) > 0 {
		target = args[0]
	}

	cmd := exec.Command("go", "test", "-v", "-count=1", target)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// SetServeFunc sets the function called by the "serve" command.
func (c *CLI) SetServeFunc(fn func() error) {
	for _, cmd := range c.root.Commands() {
		if cmd.Use == "serve" {
			cmd.RunE = func(cmd *cobra.Command, args []string) error {
				IsServeCommand = true
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

// Deprecated: Use ClientForCommand instead.
// ResolveServerWithAutoServe returns a server URL, starting an ephemeral
// server if needed. Call the returned cleanup function when done.
//
// Priority: --server flag → keychain → auto-serve → error
//
// Usage in CLI commands:
//
//	serverURL, cleanup, err := appcli.ResolveServerWithAutoServe(cmd, "myapp")
//	if err != nil { return err }
//	defer cleanup()
func ResolveServerWithAutoServe(cmd *cobra.Command, appName string) (serverURL string, cleanup func(), err error) {
	serverFlag, _ := cmd.Flags().GetString("server")
	serverURL = ResolveServerURL(serverFlag, appName)

	// If we got a URL from flag or keychain, use it (no auto-serve)
	if serverFlag != "" || serverURL != "http://localhost:3000" {
		return serverURL, func() {}, nil
	}

	// Auto-serve: start an ephemeral server if we have a handler
	if AutoServeHandler != nil {
		url, stop, err := AutoServe(AutoServeHandler)
		if err != nil {
			return "", nil, fmt.Errorf("auto-serve failed: %w", err)
		}
		return url, stop, nil
	}

	// Fall back to default URL (server may already be running)
	return serverURL, func() {}, nil
}
