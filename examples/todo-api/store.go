package main

import (
	"time"

	"github.com/google/uuid"
	"github.com/michaelwinser/appbase/db"
	"github.com/michaelwinser/appbase/store"
)

// TodoEntity is the store representation with store tags.
type TodoEntity struct {
	ID        string `json:"id"        store:"id,pk"`
	UserID    string `json:"userId"    store:"user_id,index"`
	Title     string `json:"title"     store:"title"`
	Done      bool   `json:"done"      store:"done"`
	CreatedAt string `json:"createdAt" store:"created_at"`
}

// TodoStore handles todo persistence.
type TodoStore struct {
	coll *store.Collection[TodoEntity]
}

func NewTodoStore(d *db.DB) (*TodoStore, error) {
	coll, err := store.NewCollection[TodoEntity](d, "todos")
	if err != nil {
		return nil, err
	}
	return &TodoStore{coll: coll}, nil
}

func (s *TodoStore) List(userID string) ([]TodoEntity, error) {
	return s.coll.Where("user_id", "==", userID).OrderBy("created_at", store.Desc).All()
}

func (s *TodoStore) Create(userID, title string) (*TodoEntity, error) {
	todo := &TodoEntity{
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

func (s *TodoStore) Delete(id string) error {
	return s.coll.Delete(id)
}
