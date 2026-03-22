package main

import (
	"fmt"
	"io"
	"log"
	"time"

	"github.com/michaelwinser/appbase/auth"
	"github.com/michaelwinser/appbase/db"
	"github.com/spf13/cobra"
)

func testSessionCmd() *cobra.Command {
	var email string
	var dbPath string
	var ttl time.Duration

	cmd := &cobra.Command{
		Use:   "test-session",
		Short: "Create a test session and print the cookie value",
		Long: `Creates a session in the local database for E2E testing.
Prints the session ID that can be used as the app_session cookie value.

Example usage in a test script:
  SESSION=$(appbase test-session --db test.db --email test@example.com)
  curl -H "Cookie: app_session=$SESSION" http://localhost:3000/api/todos
  rm test.db  # cleanup`,
		RunE: func(cmd *cobra.Command, args []string) error {
			log.SetOutput(io.Discard)
			if dbPath != "" {
				applyDBFlag(cmd)
			}

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

			fmt.Print(session.ID)
			return nil
		},
	}

	cmd.Flags().StringVar(&email, "email", "test@example.com", "Email for the test session")
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database path (overrides SQLITE_DB_PATH)")
	cmd.Flags().DurationVar(&ttl, "ttl", 1*time.Hour, "Session time-to-live")

	return cmd
}
