package main

import (
	"context"
	"fmt"
	"sort"

	"github.com/michaelwinser/appbase/db"
	"google.golang.org/api/iterator"
)

const todosCollection = "todos"

type firestoreTodoBackend struct {
	db *db.DB
}

// firestoreTodo is the Firestore document representation.
type firestoreTodo struct {
	UserID    string `firestore:"user_id"`
	Title     string `firestore:"title"`
	Done      bool   `firestore:"done"`
	CreatedAt string `firestore:"created_at"`
}

func (b *firestoreTodoBackend) List(userID string) ([]Todo, error) {
	ctx := context.Background()
	col := b.db.Firestore().Collection(todosCollection)
	// Single-field Where avoids needing a composite index.
	// Sort in memory since todo lists are small.
	iter := col.Where("user_id", "==", userID).Documents(ctx)
	defer iter.Stop()

	var todos []Todo
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("listing todos: %w", err)
		}
		var ft firestoreTodo
		if err := doc.DataTo(&ft); err != nil {
			return nil, fmt.Errorf("decoding todo: %w", err)
		}
		todos = append(todos, Todo{
			ID:        doc.Ref.ID,
			UserID:    ft.UserID,
			Title:     ft.Title,
			Done:      ft.Done,
			CreatedAt: ft.CreatedAt,
		})
	}
	if todos == nil {
		todos = []Todo{}
	}
	// Sort newest first (descending by created_at)
	sort.Slice(todos, func(i, j int) bool {
		return todos[i].CreatedAt > todos[j].CreatedAt
	})
	return todos, nil
}

func (b *firestoreTodoBackend) Create(todo *Todo) error {
	ctx := context.Background()
	doc := firestoreTodo{
		UserID:    todo.UserID,
		Title:     todo.Title,
		Done:      todo.Done,
		CreatedAt: todo.CreatedAt,
	}
	_, err := b.db.Firestore().Collection(todosCollection).Doc(todo.ID).Set(ctx, doc)
	if err != nil {
		return fmt.Errorf("creating todo: %w", err)
	}
	return nil
}
