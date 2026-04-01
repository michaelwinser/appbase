// Package store provides a generic entity persistence layer for appbase applications.
//
// It abstracts over SQLite and Firestore so apps define entities once and get
// CRUD on both backends. Designed for low-volume apps with few users.
//
// Usage:
//
//	type Todo struct {
//	    ID        string `json:"id"        store:"id,pk"`
//	    UserID    string `json:"userId"    store:"user_id,index"`
//	    Title     string `json:"title"     store:"title"`
//	    Done      bool   `json:"done"      store:"done"`
//	    CreatedAt string `json:"createdAt" store:"created_at"`
//	}
//
//	coll, err := store.NewCollection[Todo](db, "todos")
//	todos, err := coll.Where("user_id", "==", userID).OrderBy("created_at", store.Desc).All()
package store

import (
	"fmt"
	"regexp"

	appdb "github.com/michaelwinser/appbase/db"
)

// validIdentifier matches safe SQL identifiers (alphanumeric + underscore).
var validIdentifier = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// validOps is the set of allowed comparison operators for Where clauses.
var validOps = map[string]bool{
	"==": true, "=": true, "!=": true,
	"<": true, ">": true, "<=": true, ">=": true,
}

// Direction for OrderBy.
type Direction int

const (
	Asc  Direction = iota
	Desc
)

// backend is the internal interface implemented by SQL and Firestore.
type backend[T any] interface {
	init() error
	get(id string) (*T, error)
	create(entity *T) error
	update(id string, entity *T) error
	delete(id string) error
	query(wheres []whereClause, orderBy *orderByClause, limit int) ([]T, error)
}

type whereClause struct {
	Field string
	Op    string
	Value interface{}
}

type orderByClause struct {
	Field string
	Dir   Direction
}

// Collection provides typed CRUD operations for an entity.
type Collection[T any] struct {
	backend backend[T]
	meta    *structMeta
}

// NewCollection creates a collection backed by the given database.
// For SQL backends, it auto-creates the table and indexes.
// For Firestore, initialization is a no-op (schemaless).
func NewCollection[T any](db *appdb.DB, name string) (*Collection[T], error) {
	if !validIdentifier.MatchString(name) {
		return nil, fmt.Errorf("store: invalid collection name %q (must be alphanumeric/underscore)", name)
	}

	meta, err := parseStruct[T]()
	if err != nil {
		return nil, err
	}

	var b backend[T]
	if db.IsSQL() {
		b = &sqlBackend[T]{db: db, name: name, meta: meta}
	} else {
		b = &firestoreBackend[T]{db: db, name: name, meta: meta}
	}

	if err := b.init(); err != nil {
		return nil, fmt.Errorf("store: initializing %s: %w", name, err)
	}

	return &Collection[T]{backend: b, meta: meta}, nil
}

// Get retrieves an entity by primary key. Returns nil if not found.
func (c *Collection[T]) Get(id string) (*T, error) {
	return c.backend.get(id)
}

// Create inserts a new entity. The primary key field must be set.
func (c *Collection[T]) Create(entity *T) error {
	return c.backend.create(entity)
}

// Update replaces an existing entity. The id parameter identifies the record.
func (c *Collection[T]) Update(id string, entity *T) error {
	return c.backend.update(id, entity)
}

// Delete removes an entity by primary key.
func (c *Collection[T]) Delete(id string) error {
	return c.backend.delete(id)
}

// ReadOnly returns a read-only view of the collection.
// The returned value supports Get, All, Where, OrderBy, Limit, and First
// but not Create, Update, or Delete. Use this to enforce read-only access
// at the type level for shared/public request handling.
func (c *Collection[T]) ReadOnly() *ReadOnlyCollection[T] {
	return &ReadOnlyCollection[T]{coll: c}
}

// ReadOnlyCollection provides read-only access to a Collection.
// Write operations (Create, Update, Delete) are not available on this type.
type ReadOnlyCollection[T any] struct {
	coll *Collection[T]
}

// Get retrieves an entity by primary key. Returns nil if not found.
func (r *ReadOnlyCollection[T]) Get(id string) (*T, error) {
	return r.coll.Get(id)
}

// All returns all entities in the collection.
func (r *ReadOnlyCollection[T]) All() ([]T, error) {
	return r.coll.All()
}

// Where starts a read-only query with a filter condition.
func (r *ReadOnlyCollection[T]) Where(field, op string, value interface{}) *ReadOnlyQuery[T] {
	return &ReadOnlyQuery[T]{q: r.coll.Where(field, op, value)}
}

// ReadOnlyQuery is a read-only query builder.
type ReadOnlyQuery[T any] struct {
	q *Query[T]
}

// Where adds an additional filter (AND).
func (r *ReadOnlyQuery[T]) Where(field, op string, value interface{}) *ReadOnlyQuery[T] {
	return &ReadOnlyQuery[T]{q: r.q.Where(field, op, value)}
}

// OrderBy sets the sort order.
func (r *ReadOnlyQuery[T]) OrderBy(field string, dir Direction) *ReadOnlyQuery[T] {
	return &ReadOnlyQuery[T]{q: r.q.OrderBy(field, dir)}
}

// Limit caps the number of results.
func (r *ReadOnlyQuery[T]) Limit(n int) *ReadOnlyQuery[T] {
	return &ReadOnlyQuery[T]{q: r.q.Limit(n)}
}

// All executes the query and returns matching entities.
func (r *ReadOnlyQuery[T]) All() ([]T, error) {
	return r.q.All()
}

// First executes the query and returns the first matching entity, or nil.
func (r *ReadOnlyQuery[T]) First() (*T, error) {
	return r.q.First()
}

// hasColumn returns true if field matches a known store-tagged column name.
func (m *structMeta) hasColumn(field string) bool {
	for _, fi := range m.Fields {
		if fi.Column == field {
			return true
		}
	}
	return false
}

// validateWhere checks that field and op are safe for use in queries.
func (q *Query[T]) validateWhere(field, op string) error {
	if !q.coll.meta.hasColumn(field) {
		return fmt.Errorf("store: unknown field %q in Where (valid: %s)", field, q.coll.meta.columnNames())
	}
	if !validOps[op] {
		return fmt.Errorf("store: invalid operator %q in Where", op)
	}
	return nil
}

// columnNames returns a comma-separated list of valid column names for error messages.
func (m *structMeta) columnNames() string {
	names := make([]string, len(m.Fields))
	for i, fi := range m.Fields {
		names[i] = fi.Column
	}
	return fmt.Sprintf("[%s]", fmt.Sprintf("%s", names))
}

// All returns all entities in the collection (no filters).
func (c *Collection[T]) All() ([]T, error) {
	return c.backend.query(nil, nil, 0)
}

// Where starts a query with a filter condition.
// Supported operators: "==", "!=", "<", ">", "<=", ">=".
// The field must match a store-tagged column name on the entity struct.
func (c *Collection[T]) Where(field, op string, value interface{}) *Query[T] {
	q := &Query[T]{coll: c}
	if err := q.validateWhere(field, op); err != nil {
		q.err = err
		return q
	}
	q.wheres = []whereClause{{Field: field, Op: op, Value: value}}
	return q
}

// Query builds a filtered/sorted query against a Collection.
// Immutable — each method returns a new Query.
type Query[T any] struct {
	coll    *Collection[T]
	wheres  []whereClause
	orderBy *orderByClause
	limit   int
	err     error // sticky validation error
}

// Where adds an additional filter (AND).
func (q *Query[T]) Where(field, op string, value interface{}) *Query[T] {
	newQ := *q
	if newQ.err != nil {
		return &newQ
	}
	if err := newQ.validateWhere(field, op); err != nil {
		newQ.err = err
		return &newQ
	}
	newQ.wheres = append(append([]whereClause{}, q.wheres...), whereClause{Field: field, Op: op, Value: value})
	return &newQ
}

// OrderBy sets the sort order. For Firestore, sorting happens in memory
// to avoid composite index requirements.
func (q *Query[T]) OrderBy(field string, dir Direction) *Query[T] {
	newQ := *q
	if newQ.err != nil {
		return &newQ
	}
	if !q.coll.meta.hasColumn(field) {
		newQ.err = fmt.Errorf("store: unknown field %q in OrderBy", field)
		return &newQ
	}
	newQ.orderBy = &orderByClause{Field: field, Dir: dir}
	return &newQ
}

// Limit caps the number of results.
func (q *Query[T]) Limit(n int) *Query[T] {
	newQ := *q
	newQ.limit = n
	return &newQ
}

// All executes the query and returns matching entities.
func (q *Query[T]) All() ([]T, error) {
	if q.err != nil {
		return nil, q.err
	}
	return q.coll.backend.query(q.wheres, q.orderBy, q.limit)
}

// First executes the query and returns the first matching entity, or nil.
func (q *Query[T]) First() (*T, error) {
	if q.err != nil {
		return nil, q.err
	}
	results, err := q.Limit(1).All()
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	return &results[0], nil
}
