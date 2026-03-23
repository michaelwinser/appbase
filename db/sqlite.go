package db

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

func newSQLiteWithPath(dbPath string) (*DB, error) {
	// Ensure parent directory exists (SQLite creates the file but not directories).
	if dir := filepath.Dir(dbPath); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("creating database directory %s: %w", dir, err)
		}
	}

	sqlDB, err := sql.Open("sqlite", dbPath+"?_foreign_keys=on&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("opening sqlite database: %w", err)
	}

	// Enable WAL mode for concurrent read/write access.
	// Without WAL, a concurrent writer (e.g. async DeleteExpired) blocks
	// readers, causing SQLITE_BUSY even with _busy_timeout in some cases.
	if _, err := sqlDB.Exec("PRAGMA journal_mode=WAL"); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("enabling WAL mode: %w", err)
	}

	log.Printf("Using SQLite store (path: %s)", dbPath)
	return &DB{sql: sqlDB, storeType: "sqlite"}, nil
}
