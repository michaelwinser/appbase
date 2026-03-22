package db

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/mattn/go-sqlite3"
)

func newSQLiteWithPath(dbPath string) (*DB, error) {
	sqlDB, err := sql.Open("sqlite3", dbPath+"?_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("opening sqlite database: %w", err)
	}

	log.Printf("Using SQLite store (path: %s)", dbPath)
	return &DB{sql: sqlDB, storeType: "sqlite"}, nil
}
