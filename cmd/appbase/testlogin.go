package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/michaelwinser/appbase/auth"
	"github.com/michaelwinser/appbase/db"
	"github.com/zalando/go-keyring"

	"github.com/spf13/cobra"
)

const keychainServicePrefix = "appbase/"

func testLoginCmd() *cobra.Command {
	var email string
	var serverURL string
	var appName string
	var ttl time.Duration

	cmd := &cobra.Command{
		Use:   "test-login",
		Short: "Create a test session in DB and keychain (for CLI e2e tests)",
		Long: `Seeds both the database and OS keychain so CLI commands work
without a browser OAuth flow. For E2E testing only.

Example:
  ./myapp serve &
  appbase test-login --server http://localhost:3000 --app myapp
  ./myapp list
  ./myapp add "Test item"
  appbase test-logout --app myapp`,
		RunE: func(cmd *cobra.Command, args []string) error {
			log.SetOutput(io.Discard)

			// Resolve app name from flag or project config
			if appName == "" {
				if p, err := loadProject(); err == nil {
					appName = p.Name
				} else {
					return fmt.Errorf("--app is required (no app.yaml/app.json found)")
				}
			}

			// Resolve server URL
			if serverURL == "" {
				if p, err := loadProject(); err == nil {
					serverURL = fmt.Sprintf("http://localhost:%d", p.Port)
				} else {
					serverURL = "http://localhost:3000"
				}
			}

			// Connect to the database and create a session
			database, err := db.New()
			if err != nil {
				return fmt.Errorf("connecting to database: %w", err)
			}
			defer database.Close()

			sessions, err := auth.NewSessionStore(database)
			if err != nil {
				return fmt.Errorf("initializing session store: %w", err)
			}

			session, err := sessions.Create(email, email, ttl)
			if err != nil {
				return fmt.Errorf("creating session: %w", err)
			}

			// Store in keychain (same keys that cli/auth.go uses)
			service := keychainServicePrefix + appName
			if err := keyring.Set(service, "cli-session", session.ID); err != nil {
				return fmt.Errorf("storing session in keychain: %w", err)
			}
			if err := keyring.Set(service, "cli-server", serverURL); err != nil {
				return fmt.Errorf("storing server URL in keychain: %w", err)
			}

			fmt.Fprintf(os.Stderr, "Test login: %s → %s (session in DB + keychain)\n", email, serverURL)
			return nil
		},
	}

	cmd.Flags().StringVar(&email, "email", "test@example.com", "Email for the test session")
	cmd.Flags().StringVar(&serverURL, "server", "", "Server URL (default: from app.yaml or localhost:3000)")
	cmd.Flags().StringVar(&appName, "app", "", "App name for keychain (default: from app.yaml)")
	cmd.Flags().DurationVar(&ttl, "ttl", 1*time.Hour, "Session time-to-live")

	return cmd
}

func testLogoutCmd() *cobra.Command {
	var appName string

	cmd := &cobra.Command{
		Use:   "test-logout",
		Short: "Clear test session from keychain (cleanup after e2e tests)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if appName == "" {
				if p, err := loadProject(); err == nil {
					appName = p.Name
				} else {
					return fmt.Errorf("--app is required (no app.yaml/app.json found)")
				}
			}

			service := keychainServicePrefix + appName
			keyring.Delete(service, "cli-session")
			keyring.Delete(service, "cli-server")
			fmt.Fprintf(os.Stderr, "Test logout: cleared keychain for %s\n", appName)
			return nil
		},
	}

	cmd.Flags().StringVar(&appName, "app", "", "App name for keychain (default: from app.yaml)")
	return cmd
}
