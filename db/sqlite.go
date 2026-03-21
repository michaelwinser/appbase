package db

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	_ "github.com/mattn/go-sqlite3"
)

func newSQLite() (*DB, error) {
	dbPath := os.Getenv("SQLITE_DB_PATH")
	if dbPath == "" {
		dbPath = "data/app.db"
	}

	sqlDB, err := sql.Open("sqlite3", dbPath+"?_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("opening sqlite database: %w", err)
	}

	log.Printf("Using SQLite store (path: %s)", dbPath)
	return &DB{sql: sqlDB, storeType: "sqlite"}, nil
}
