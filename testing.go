// Testing helpers for appbase applications.
//
// This file provides utilities for testing apps built on appbase.
// It is only compiled in test binaries (despite being in the main package,
// the helpers are designed to be called from _test.go files).
package appbase

import (
	"os"

	"github.com/michaelwinser/appbase/db"
)

// NewTestDB creates an in-memory SQLite database for testing.
// The caller should defer db.Close().
func NewTestDB() (*db.DB, error) {
	os.Setenv("STORE_TYPE", "sqlite")
	os.Setenv("SQLITE_DB_PATH", ":memory:")
	return db.New()
}
