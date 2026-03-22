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

	appdb "github.com/michaelwinser/appbase/db"
)

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

// All returns all entities in the collection (no filters).
func (c *Collection[T]) All() ([]T, error) {
	return c.backend.query(nil, nil, 0)
}

// Where starts a query with a filter condition.
// Supported operators: "==", "!=", "<", ">", "<=", ">=".
func (c *Collection[T]) Where(field, op string, value interface{}) *Query[T] {
	return &Query[T]{
		coll:   c,
		wheres: []whereClause{{Field: field, Op: op, Value: value}},
	}
}

// Query builds a filtered/sorted query against a Collection.
// Immutable — each method returns a new Query.
type Query[T any] struct {
	coll    *Collection[T]
	wheres  []whereClause
	orderBy *orderByClause
	limit   int
}

// Where adds an additional filter (AND).
func (q *Query[T]) Where(field, op string, value interface{}) *Query[T] {
	newQ := *q
	newQ.wheres = append(append([]whereClause{}, q.wheres...), whereClause{Field: field, Op: op, Value: value})
	return &newQ
}

// OrderBy sets the sort order. For Firestore, sorting happens in memory
// to avoid composite index requirements.
func (q *Query[T]) OrderBy(field string, dir Direction) *Query[T] {
	newQ := *q
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
	return q.coll.backend.query(q.wheres, q.orderBy, q.limit)
}

// First executes the query and returns the first matching entity, or nil.
func (q *Query[T]) First() (*T, error) {
	results, err := q.Limit(1).All()
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	return &results[0], nil
}
