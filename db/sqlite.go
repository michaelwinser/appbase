package db

import (
	"database/sql"
	"fmt"
	"log"

	_ "modernc.org/sqlite"
)

func newSQLiteWithPath(dbPath string) (*DB, error) {
	sqlDB, err := sql.Open("sqlite", dbPath+"?_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("opening sqlite database: %w", err)
	}

	log.Printf("Using SQLite store (path: %s)", dbPath)
	return &DB{sql: sqlDB, storeType: "sqlite"}, nil
}
