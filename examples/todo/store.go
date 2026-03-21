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

// TodoStore handles todo persistence using the appbase DB connection.
type TodoStore struct {
	db *db.DB
}

// List returns all todos for a user.
func (s *TodoStore) List(userID string) ([]Todo, error) {
	rows, err := s.db.Query(
		`SELECT id, user_id, title, done, created_at FROM todos WHERE user_id = ? ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var todos []Todo
	for rows.Next() {
		var t Todo
		var done int
		if err := rows.Scan(&t.ID, &t.UserID, &t.Title, &done, &t.CreatedAt); err != nil {
			return nil, err
		}
		t.Done = done != 0
		todos = append(todos, t)
	}
	if todos == nil {
		todos = []Todo{}
	}
	return todos, rows.Err()
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
	_, err := s.db.Exec(
		`INSERT INTO todos (id, user_id, title, done, created_at) VALUES (?, ?, ?, ?, ?)`,
		todo.ID, todo.UserID, todo.Title, 0, todo.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return todo, nil
}
