// Package db provides database connection management for appbase applications.
//
// Applications use the DB type to interact with their database.
// appbase manages the connection lifecycle and runs migrations.
//
// For SQL backends (SQLite, Postgres), use Exec/Query/QueryRow.
// For Firestore, use the Firestore() client directly.
// Check StoreType() to determine which backend is active.
package db

import (
	"database/sql"
	"fmt"
	"os"

	"cloud.google.com/go/firestore"
)

// DB wraps a database connection and provides common operations.
// Holds either a SQL connection or a Firestore client, depending on STORE_TYPE.
type DB struct {
	sql       *sql.DB
	firestore *firestore.Client
	storeType string
}

// SQL returns the underlying *sql.DB for direct access.
// Returns nil when using Firestore.
func (d *DB) SQL() *sql.DB {
	return d.sql
}

// Firestore returns the underlying Firestore client.
// Returns nil when using a SQL backend.
func (d *DB) Firestore() *firestore.Client {
	return d.firestore
}

// StoreType returns the active store type ("sqlite", "firestore", etc.).
func (d *DB) StoreType() string {
	return d.storeType
}

// IsSQL returns true if the active backend is SQL-based (SQLite, Postgres).
func (d *DB) IsSQL() bool {
	return d.sql != nil
}

// Close closes the database connection.
func (d *DB) Close() error {
	if d.sql != nil {
		return d.sql.Close()
	}
	if d.firestore != nil {
		return d.firestore.Close()
	}
	return nil
}

// Exec executes a SQL query without returning rows.
// Returns an error if called on a Firestore backend.
func (d *DB) Exec(query string, args ...interface{}) (sql.Result, error) {
	if d.sql == nil {
		return nil, fmt.Errorf("Exec not supported on %s backend", d.storeType)
	}
	return d.sql.Exec(query, args...)
}

// Query executes a SQL query that returns rows.
// Returns an error if called on a Firestore backend.
func (d *DB) Query(query string, args ...interface{}) (*sql.Rows, error) {
	if d.sql == nil {
		return nil, fmt.Errorf("Query not supported on %s backend", d.storeType)
	}
	return d.sql.Query(query, args...)
}

// QueryRow executes a SQL query that returns at most one row.
// Panics if called on a Firestore backend — use IsSQL() to check first.
func (d *DB) QueryRow(query string, args ...interface{}) *sql.Row {
	if d.sql == nil {
		panic(fmt.Sprintf("QueryRow not supported on %s backend — use IsSQL() to check before calling", d.storeType))
	}
	return d.sql.QueryRow(query, args...)
}

// Begin starts a SQL transaction.
// Returns an error if called on a Firestore backend.
func (d *DB) Begin() (*sql.Tx, error) {
	if d.sql == nil {
		return nil, fmt.Errorf("Begin not supported on %s backend", d.storeType)
	}
	return d.sql.Begin()
}

// DBConfig configures the database connection.
type DBConfig struct {
	// StoreType selects the backend: "sqlite" (default) or "firestore".
	StoreType string
	// SQLitePath is the SQLite database file path (default: "data/app.db").
	SQLitePath string
	// GCPProject is required for the Firestore backend.
	GCPProject string
}

// New creates a new DB connection.
// Accepts an optional DBConfig; falls back to environment variables if not provided.
func New(configs ...DBConfig) (*DB, error) {
	var cfg DBConfig
	if len(configs) > 0 {
		cfg = configs[0]
	}

	// Fall back to env vars for unset fields
	if cfg.StoreType == "" {
		cfg.StoreType = os.Getenv("STORE_TYPE")
	}
	if cfg.StoreType == "" {
		cfg.StoreType = "sqlite"
	}
	if cfg.SQLitePath == "" {
		cfg.SQLitePath = os.Getenv("SQLITE_DB_PATH")
	}
	if cfg.SQLitePath == "" {
		cfg.SQLitePath = "data/app.db"
	}
	if cfg.GCPProject == "" {
		cfg.GCPProject = os.Getenv("GOOGLE_CLOUD_PROJECT")
	}

	switch cfg.StoreType {
	case "sqlite":
		return newSQLiteWithPath(cfg.SQLitePath)
	case "firestore":
		return newFirestoreWithProject(cfg.GCPProject)
	default:
		return nil, fmt.Errorf("unsupported store type: %q (supported: sqlite, firestore)", cfg.StoreType)
	}
}

// NewFromSQL creates a DB wrapping an existing *sql.DB connection.
// Useful for testing with in-memory databases.
func NewFromSQL(sqlDB *sql.DB) *DB {
	return &DB{sql: sqlDB, storeType: "sqlite"}
}

// Migrate runs SQL migration statements against the database.
// For SQL backends, uses CREATE TABLE IF NOT EXISTS pattern — safe to run repeatedly.
// For Firestore, this is a no-op (Firestore is schemaless).
func (d *DB) Migrate(schema string) error {
	if d.sql == nil {
		// Firestore is schemaless — migrations are a no-op
		return nil
	}
	_, err := d.sql.Exec(schema)
	if err != nil {
		return fmt.Errorf("running migration: %w", err)
	}
	return nil
}
