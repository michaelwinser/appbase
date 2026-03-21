---
name: add-entity
description: Add a new domain entity to an app built on appbase
trigger: When the user wants to add a new data type/entity to their application
---

# Adding a New Entity

When the app needs a new data type (e.g., Activity, Location, Contact), follow this checklist to add it consistently across all layers. Every entity needs **4 files** to work on both SQLite and Firestore.

## Firestore Constraints

Firestore auto-indexes single fields. Queries that combine multiple conditions or use inequality + orderBy on different fields require composite indexes (created manually in the Console). To keep things working out of the box:

- **DO**: `Where("user_id", "==", ...)` — single equality filter (auto-indexed)
- **DO**: `Where("user_id", "==", ...).Where("done", "==", ...)` — equality on same field works
- **AVOID**: `Where("user_id", "==", ...).OrderBy("created_at")` — requires composite index
- **AVOID**: `Where("field", "<", ...).OrderBy("other_field")` — requires composite index
- **Instead**: filter by single field, sort in memory for small collections

If a composite index is truly needed, document it and add it to `firestore.indexes.json`.

## Checklist

### 1. Define the SQL Schema

Add to your `schema.go` (or migration file):

```sql
CREATE TABLE IF NOT EXISTS activities (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    -- your fields here
    created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_activities_user ON activities(user_id);
```

**Rules:**
- Always include `id TEXT PRIMARY KEY` and `user_id TEXT NOT NULL`
- Always include `created_at` (and `updated_at` if mutable)
- Add indexes on `user_id` and any frequently queried columns
- Use `TEXT` for dates (format: RFC3339)
- Use `TEXT` for UUIDs
- Firestore ignores this schema (it's schemaless) — `Migrate()` is a no-op

### 2. Create store.go — Interface and Factory

```go
package main

import (
    "time"
    "github.com/google/uuid"
    "github.com/michaelwinser/appbase/db"
)

type Activity struct {
    ID        string `json:"id"`
    UserID    string `json:"userId"`
    Name      string `json:"name"`
    CreatedAt string `json:"createdAt"`
}

// activityBackend abstracts persistence across SQL and Firestore.
type activityBackend interface {
    List(userID string) ([]Activity, error)
    Get(userID, id string) (*Activity, error)
    Create(a *Activity) error
    Delete(userID, id string) error
}

type ActivityStore struct {
    backend activityBackend
}

func NewActivityStore(d *db.DB) *ActivityStore {
    if d.IsSQL() {
        return &ActivityStore{backend: &sqlActivityBackend{db: d}}
    }
    return &ActivityStore{backend: &firestoreActivityBackend{db: d}}
}

func (s *ActivityStore) List(userID string) ([]Activity, error) {
    return s.backend.List(userID)
}

func (s *ActivityStore) Get(userID, id string) (*Activity, error) {
    return s.backend.Get(userID, id)
}

func (s *ActivityStore) Create(userID, name string) (*Activity, error) {
    a := &Activity{
        ID:        uuid.New().String(),
        UserID:    userID,
        Name:      name,
        CreatedAt: time.Now().Format(time.RFC3339),
    }
    if err := s.backend.Create(a); err != nil {
        return nil, err
    }
    return a, nil
}

func (s *ActivityStore) Delete(userID, id string) error {
    return s.backend.Delete(userID, id)
}
```

### 3. Create store_sql.go — SQL Backend

```go
package main

import (
    "database/sql"
    "github.com/michaelwinser/appbase/db"
)

type sqlActivityBackend struct {
    db *db.DB
}

func (b *sqlActivityBackend) List(userID string) ([]Activity, error) {
    rows, err := b.db.Query(
        `SELECT id, user_id, name, created_at FROM activities
         WHERE user_id = ? ORDER BY created_at DESC`, userID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var items []Activity
    for rows.Next() {
        var a Activity
        if err := rows.Scan(&a.ID, &a.UserID, &a.Name, &a.CreatedAt); err != nil {
            return nil, err
        }
        items = append(items, a)
    }
    if items == nil {
        items = []Activity{}
    }
    return items, rows.Err()
}

func (b *sqlActivityBackend) Get(userID, id string) (*Activity, error) {
    var a Activity
    err := b.db.QueryRow(
        `SELECT id, user_id, name, created_at FROM activities
         WHERE id = ? AND user_id = ?`, id, userID,
    ).Scan(&a.ID, &a.UserID, &a.Name, &a.CreatedAt)
    if err == sql.ErrNoRows {
        return nil, nil
    }
    return &a, err
}

func (b *sqlActivityBackend) Create(a *Activity) error {
    _, err := b.db.Exec(
        `INSERT INTO activities (id, user_id, name, created_at) VALUES (?, ?, ?, ?)`,
        a.ID, a.UserID, a.Name, a.CreatedAt)
    return err
}

func (b *sqlActivityBackend) Delete(userID, id string) error {
    _, err := b.db.Exec(
        `DELETE FROM activities WHERE id = ? AND user_id = ?`, id, userID)
    return err
}
```

### 4. Create store_firestore.go — Firestore Backend

```go
package main

import (
    "context"
    "fmt"
    "sort"

    "github.com/michaelwinser/appbase/db"
    "google.golang.org/api/iterator"
    "google.golang.org/grpc/codes"
    "google.golang.org/grpc/status"
)

const activitiesCollection = "activities"

type firestoreActivityBackend struct {
    db *db.DB
}

type firestoreActivity struct {
    UserID    string `firestore:"user_id"`
    Name      string `firestore:"name"`
    CreatedAt string `firestore:"created_at"`
}

func (b *firestoreActivityBackend) List(userID string) ([]Activity, error) {
    ctx := context.Background()
    // Single-field query — no composite index needed
    iter := b.db.Firestore().Collection(activitiesCollection).
        Where("user_id", "==", userID).Documents(ctx)
    defer iter.Stop()

    var items []Activity
    for {
        doc, err := iter.Next()
        if err == iterator.Done {
            break
        }
        if err != nil {
            return nil, fmt.Errorf("listing activities: %w", err)
        }
        var fa firestoreActivity
        if err := doc.DataTo(&fa); err != nil {
            return nil, fmt.Errorf("decoding activity: %w", err)
        }
        items = append(items, Activity{
            ID: doc.Ref.ID, UserID: fa.UserID,
            Name: fa.Name, CreatedAt: fa.CreatedAt,
        })
    }
    if items == nil {
        items = []Activity{}
    }
    // Sort in memory — avoids composite index requirement
    sort.Slice(items, func(i, j int) bool {
        return items[i].CreatedAt > items[j].CreatedAt
    })
    return items, nil
}

func (b *firestoreActivityBackend) Get(userID, id string) (*Activity, error) {
    ctx := context.Background()
    doc, err := b.db.Firestore().Collection(activitiesCollection).Doc(id).Get(ctx)
    if err != nil {
        if status.Code(err) == codes.NotFound {
            return nil, nil
        }
        return nil, err
    }
    var fa firestoreActivity
    if err := doc.DataTo(&fa); err != nil {
        return nil, err
    }
    // Verify user_id matches (Firestore doesn't enforce this in Get)
    if fa.UserID != userID {
        return nil, nil
    }
    return &Activity{
        ID: doc.Ref.ID, UserID: fa.UserID,
        Name: fa.Name, CreatedAt: fa.CreatedAt,
    }, nil
}

func (b *firestoreActivityBackend) Create(a *Activity) error {
    ctx := context.Background()
    doc := firestoreActivity{
        UserID: a.UserID, Name: a.Name, CreatedAt: a.CreatedAt,
    }
    _, err := b.db.Firestore().Collection(activitiesCollection).Doc(a.ID).Set(ctx, doc)
    return err
}

func (b *firestoreActivityBackend) Delete(userID, id string) error {
    // Verify ownership before deleting
    a, err := b.Get(userID, id)
    if err != nil {
        return err
    }
    if a == nil {
        return nil // not found or wrong user — no-op
    }
    ctx := context.Background()
    _, err = b.db.Firestore().Collection(activitiesCollection).Doc(id).Delete(ctx)
    return err
}
```

### 5. Add HTTP Handlers

```go
r.Get("/api/activities", listActivitiesHandler)
r.Post("/api/activities", createActivityHandler)
r.Get("/api/activities/{id}", getActivityHandler)
r.Delete("/api/activities/{id}", deleteActivityHandler)
```

**Rules:**
- Use `appbase.UserID(r)` for the authenticated user
- Use `server.RespondJSON` and `server.RespondError`
- Validate input before calling the store
- Return 404 for not found, 400 for bad input, 201 for created

### 6. Add CLI Commands

```go
cli.AddCommand(cli.Command("activities", "List activities", listActivitiesCmd))
cli.AddCommand(cli.Command("add-activity", "Add an activity", addActivityCmd))
```

### 7. Write Use Case Tests

For each operation, add a numbered use case test:

```go
h.Run("UC-XXXX", "Create an activity", func(c *harness.Client) {
    login(c)
    resp := c.POST("/api/activities", `{...}`)
    c.AssertStatus(resp, 201)
})
```

Map each test to a use case in the PRD.

### 8. Update the PRD

Add the new use cases to `docs/prd.md` with acceptance criteria.

### 9. Verify

```bash
go build ./...
go test -v ./...
```

## Key Differences: SQL vs Firestore

| Concern | SQL | Firestore |
|---------|-----|-----------|
| Schema | `CREATE TABLE` in schema.go | Schemaless (no-op) |
| IDs | `uuid.New().String()` | Same — stored as doc ID |
| Multi-tenancy | `WHERE user_id = ?` | `Where("user_id", "==", ...)` |
| Sorting | `ORDER BY created_at DESC` | Sort in memory (avoids index) |
| Get by ID | `WHERE id = ? AND user_id = ?` | `Doc(id).Get()` + verify user_id |
| Delete | `DELETE WHERE id = ? AND user_id = ?` | Get + verify user_id + `Doc(id).Delete()` |
| Not found | `sql.ErrNoRows` → return nil | `codes.NotFound` → return nil |
| Transactions | `db.Begin()` | `db.Firestore().RunTransaction()` |

## Anti-Patterns

- **Don't skip user_id** — every query must scope by user
- **Don't use composite Firestore queries without indexes** — stick to single-field Where, sort in memory
- **Don't put business logic in handlers** — handlers validate and delegate to store
- **Don't create entities without tests** — every store method needs a test
- **Don't forget the CLI** — if the API supports it, the CLI should too
- **Don't use `&MyStore{db: d}` directly** — always use `NewMyStore(d)` factory
