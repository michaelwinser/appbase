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

// New creates a new DB connection based on environment configuration.
// STORE_TYPE determines the backend: "sqlite" (default) or "firestore".
func New() (*DB, error) {
	storeType := os.Getenv("STORE_TYPE")
	if storeType == "" {
		storeType = "sqlite"
	}

	switch storeType {
	case "sqlite":
		return newSQLite()
	case "firestore":
		return newFirestore()
	default:
		return nil, fmt.Errorf("unsupported STORE_TYPE: %q (supported: sqlite, firestore)", storeType)
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
