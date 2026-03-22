package main

import (
	"github.com/michaelwinser/appbase/db"
)

type sqlTodoBackend struct {
	db *db.DB
}

func (b *sqlTodoBackend) List(userID string) ([]Todo, error) {
	rows, err := b.db.Query(
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

func (b *sqlTodoBackend) Create(todo *Todo) error {
	_, err := b.db.Exec(
		`INSERT INTO todos (id, user_id, title, done, created_at) VALUES (?, ?, ?, ?, ?)`,
		todo.ID, todo.UserID, todo.Title, 0, todo.CreatedAt,
	)
	return err
}
