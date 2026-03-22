package main

import (
	"time"

	"github.com/google/uuid"
	"github.com/michaelwinser/appbase/db"
)

// Todo represents a todo item.
type Todo struct {
	ID        string `json:"id"`
	UserID    string `json:"userId"`
	Title     string `json:"title"`
	Done      bool   `json:"done"`
	CreatedAt string `json:"createdAt"`
}

// todoBackend abstracts todo persistence across SQL and Firestore.
type todoBackend interface {
	List(userID string) ([]Todo, error)
	Create(todo *Todo) error
}

// TodoStore handles todo persistence using the appbase DB connection.
type TodoStore struct {
	backend todoBackend
}

// NewTodoStore creates a store backed by the given database.
func NewTodoStore(d *db.DB) *TodoStore {
	if d.IsSQL() {
		return &TodoStore{backend: &sqlTodoBackend{db: d}}
	}
	return &TodoStore{backend: &firestoreTodoBackend{db: d}}
}

// List returns all todos for a user.
func (s *TodoStore) List(userID string) ([]Todo, error) {
	return s.backend.List(userID)
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
	if err := s.backend.Create(todo); err != nil {
		return nil, err
	}
	return todo, nil
}
