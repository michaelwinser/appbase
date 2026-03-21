// Package db provides database connection management for appbase applications.
//
// Applications use the DB interface to interact with their database.
// appbase manages the connection lifecycle and runs migrations.
package db

import (
	"database/sql"
	"fmt"
	"os"
)

// DB wraps a database connection and provides common operations.
// Applications use this to build their domain-specific stores.
type DB struct {
	sql       *sql.DB
	storeType string
}

// SQL returns the underlying *sql.DB for direct access.
// Use this to build your domain store queries.
func (d *DB) SQL() *sql.DB {
	return d.sql
}

// StoreType returns the active store type ("sqlite" or "firestore").
func (d *DB) StoreType() string {
	return d.storeType
}

// Close closes the database connection.
func (d *DB) Close() error {
	if d.sql != nil {
		return d.sql.Close()
	}
	return nil
}

// Exec executes a query without returning rows.
func (d *DB) Exec(query string, args ...interface{}) (sql.Result, error) {
	return d.sql.Exec(query, args...)
}

// Query executes a query that returns rows.
func (d *DB) Query(query string, args ...interface{}) (*sql.Rows, error) {
	return d.sql.Query(query, args...)
}

// QueryRow executes a query that returns at most one row.
func (d *DB) QueryRow(query string, args ...interface{}) *sql.Row {
	return d.sql.QueryRow(query, args...)
}

// Begin starts a transaction.
func (d *DB) Begin() (*sql.Tx, error) {
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
	default:
		return nil, fmt.Errorf("unsupported STORE_TYPE: %q (supported: sqlite)", storeType)
	}
}

// Migrate runs SQL migration statements against the database.
// Uses CREATE TABLE IF NOT EXISTS pattern — safe to run repeatedly.
func (d *DB) Migrate(schema string) error {
	_, err := d.sql.Exec(schema)
	if err != nil {
		return fmt.Errorf("running migration: %w", err)
	}
	return nil
}
