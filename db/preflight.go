package db

import (
	"context"
	"fmt"
	"log"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const preflightCollection = "_appbase_preflight"
const preflightDocID = "check"

// Preflight verifies the database backend is functional by writing,
// reading, and deleting a canary record. Call this at startup to
// fail fast with a clear error instead of discovering problems via
// 500s in production.
//
// Returns nil if the check passes. Returns an error describing exactly
// what failed (connection, write, read, or delete).
func (d *DB) Preflight() error {
	log.Printf("Running preflight check for %s backend...", d.storeType)

	if d.IsSQL() {
		return d.preflightSQL()
	}
	if d.firestore != nil {
		return d.preflightFirestore()
	}
	return fmt.Errorf("preflight: no backend configured")
}

func (d *DB) preflightSQL() error {
	// Test write
	_, err := d.sql.Exec(`CREATE TABLE IF NOT EXISTS _appbase_preflight (id TEXT PRIMARY KEY, ts TEXT)`)
	if err != nil {
		return fmt.Errorf("preflight: cannot create table: %w", err)
	}

	ts := time.Now().Format(time.RFC3339)
	_, err = d.sql.Exec(`INSERT OR REPLACE INTO _appbase_preflight (id, ts) VALUES ('check', ?)`, ts)
	if err != nil {
		return fmt.Errorf("preflight: cannot write: %w", err)
	}

	// Test read
	var readBack string
	err = d.sql.QueryRow(`SELECT ts FROM _appbase_preflight WHERE id = 'check'`).Scan(&readBack)
	if err != nil {
		return fmt.Errorf("preflight: cannot read: %w", err)
	}
	if readBack != ts {
		return fmt.Errorf("preflight: read mismatch (wrote %q, got %q)", ts, readBack)
	}

	// Clean up — drop the table entirely so it doesn't persist
	_, err = d.sql.Exec(`DROP TABLE IF EXISTS _appbase_preflight`)
	if err != nil {
		return fmt.Errorf("preflight: cannot drop table: %w", err)
	}

	log.Printf("Preflight check passed (sqlite)")
	return nil
}

func (d *DB) preflightFirestore() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	col := d.firestore.Collection(preflightCollection)
	doc := col.Doc(preflightDocID)

	ts := time.Now().Format(time.RFC3339)

	// Test write
	_, err := doc.Set(ctx, map[string]interface{}{"ts": ts})
	if err != nil {
		return fmt.Errorf("preflight: cannot write to Firestore: %w", err)
	}

	// Test read
	snap, err := doc.Get(ctx)
	if err != nil {
		return fmt.Errorf("preflight: cannot read from Firestore: %w", err)
	}
	readBack, ok := snap.Data()["ts"].(string)
	if !ok || readBack != ts {
		return fmt.Errorf("preflight: read mismatch (wrote %q, got %q)", ts, readBack)
	}

	// Test query (single-field Where — should always work without index)
	iter := col.Where("ts", "==", ts).Documents(ctx)
	_, err = iter.Next()
	iter.Stop()
	if err != nil {
		// Ignore NotFound — the doc may not be indexed yet
		if status.Code(err) != codes.NotFound {
			return fmt.Errorf("preflight: query failed: %w", err)
		}
	}

	// Clean up
	_, err = doc.Delete(ctx)
	if err != nil {
		return fmt.Errorf("preflight: cannot delete from Firestore: %w", err)
	}

	log.Printf("Preflight check passed (firestore)")
	return nil
}
