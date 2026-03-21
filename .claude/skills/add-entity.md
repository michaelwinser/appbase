---
name: add-entity
description: Add a new domain entity to an app built on appbase
trigger: When the user wants to add a new data type/entity to their application
---

# Adding a New Entity

When the app needs a new data type (e.g., Activity, Location, Contact), follow this checklist to add it consistently across all layers.

## Checklist

### 1. Define the Schema

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
- Use `TEXT` for dates (format: `2006-01-02` or RFC3339)
- Use `TEXT` for UUIDs

### 2. Create the Store

In your store file, implement these standard methods:

```go
func (s *Store) ListActivities(userID string) ([]Activity, error)
func (s *Store) GetActivity(userID string, id string) (*Activity, error)
func (s *Store) CreateActivity(a *Activity) error
func (s *Store) UpdateActivity(userID string, a *Activity) error
func (s *Store) DeleteActivity(userID string, id string) error
```

**Rules:**
- Always filter by `user_id` (multi-tenancy)
- Return `nil, nil` for not found (not an error)
- Use `uuid.New().String()` for new IDs

### 3. Add HTTP Handlers

```go
r.Get("/api/activities", listActivitiesHandler)
r.Post("/api/activities", createActivityHandler)
r.Get("/api/activities/{id}", getActivityHandler)
r.Put("/api/activities/{id}", updateActivityHandler)
r.Delete("/api/activities/{id}", deleteActivityHandler)
```

**Rules:**
- Use `appbase.UserID(r)` for the authenticated user
- Use `server.RespondJSON` and `server.RespondError`
- Validate input before calling the store
- Return 404 for not found, 400 for bad input, 201 for created

### 4. Add CLI Commands

```go
cli.AddCommand(cli.Command("activities", "List activities", listActivitiesCmd))
cli.AddCommand(cli.Command("add-activity", "Add an activity", addActivityCmd))
```

### 5. Write Use Case Tests

For each operation, add a numbered use case test:

```go
h.Run("UC-XXXX", "Create an activity", func(c *harness.Client) {
    login(c)
    resp := c.POST("/api/activities", `{...}`)
    c.AssertStatus(resp, 201)
})
```

Map each test to a use case in the PRD.

### 6. Update the PRD

Add the new use cases to `docs/prd.md` with acceptance criteria.

### 7. Verify

```bash
go build ./...
go test -v ./...
```

## Anti-Patterns

- **Don't skip user_id** — every query must scope by user
- **Don't put business logic in handlers** — handlers validate and delegate to store/service
- **Don't create entities without tests** — every store method needs a test
- **Don't forget the CLI** — if the API supports it, the CLI should too
