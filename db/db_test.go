package db

import (
	"os"
	"testing"
)

func setupTestDB(t *testing.T) *DB {
	t.Helper()
	os.Setenv("STORE_TYPE", "sqlite")
	os.Setenv("SQLITE_DB_PATH", ":memory:")
	t.Cleanup(func() {
		os.Unsetenv("SQLITE_DB_PATH")
	})

	db, err := New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestNew_SQLite(t *testing.T) {
	db := setupTestDB(t)
	if db.StoreType() != "sqlite" {
		t.Fatalf("expected sqlite, got %s", db.StoreType())
	}
}

func TestMigrate_CreatesTable(t *testing.T) {
	db := setupTestDB(t)

	err := db.Migrate(`CREATE TABLE IF NOT EXISTS test (id TEXT PRIMARY KEY, name TEXT);`)
	if err != nil {
		t.Fatal(err)
	}

	// Verify table exists
	_, err = db.Exec(`INSERT INTO test (id, name) VALUES ('1', 'hello')`)
	if err != nil {
		t.Fatal("table not created:", err)
	}

	row := db.QueryRow(`SELECT name FROM test WHERE id = '1'`)
	var name string
	if err := row.Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "hello" {
		t.Fatalf("expected 'hello', got %q", name)
	}
}

func TestMigrate_IsIdempotent(t *testing.T) {
	db := setupTestDB(t)
	schema := `CREATE TABLE IF NOT EXISTS test (id TEXT PRIMARY KEY);`

	if err := db.Migrate(schema); err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(schema); err != nil {
		t.Fatal("Migrate is not idempotent:", err)
	}
}
