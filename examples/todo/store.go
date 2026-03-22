package main

import (
	"time"

	"github.com/google/uuid"
	"github.com/michaelwinser/appbase/db"
	"github.com/michaelwinser/appbase/store"
)

// Todo represents a todo item.
type Todo struct {
	ID        string `json:"id"        store:"id,pk"`
	UserID    string `json:"userId"    store:"user_id,index"`
	Title     string `json:"title"     store:"title"`
	Done      bool   `json:"done"      store:"done"`
	CreatedAt string `json:"createdAt" store:"created_at"`
}

// TodoStore handles todo persistence.
type TodoStore struct {
	coll *store.Collection[Todo]
}

// NewTodoStore creates a store backed by the given database.
// Auto-creates the table (SQLite) or is a no-op (Firestore).
func NewTodoStore(d *db.DB) (*TodoStore, error) {
	coll, err := store.NewCollection[Todo](d, "todos")
	if err != nil {
		return nil, err
	}
	return &TodoStore{coll: coll}, nil
}

// List returns all todos for a user, newest first.
func (s *TodoStore) List(userID string) ([]Todo, error) {
	return s.coll.Where("user_id", "==", userID).OrderBy("created_at", store.Desc).All()
}

// Create adds a new todo.
func (s *TodoStore) Create(userID, title string) (*Todo, error) {
	todo := &Todo{
		ID:        uuid.New().String(),
		UserID:    userID,
		Title:     title,
		Done:      false,
		CreatedAt: time.Now().Format(time.RFC3339),
	}
	if err := s.coll.Create(todo); err != nil {
		return nil, err
	}
	return todo, nil
}
