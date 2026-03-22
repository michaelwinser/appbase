package main

import (
	"time"

	"github.com/google/uuid"
	"github.com/michaelwinser/appbase/db"
	"github.com/michaelwinser/appbase/store"
)

// Bookmark represents a saved URL with a title and optional tags.
type Bookmark struct {
	ID        string `json:"id"        store:"id,pk"`
	UserID    string `json:"userId"    store:"user_id,index"`
	URL       string `json:"url"       store:"url"`
	Title     string `json:"title"     store:"title"`
	Tags      string `json:"tags"      store:"tags"`
	CreatedAt string `json:"createdAt" store:"created_at"`
}

// BookmarkStore handles bookmark persistence.
type BookmarkStore struct {
	coll *store.Collection[Bookmark]
}

// NewBookmarkStore creates a store backed by the given database.
func NewBookmarkStore(d *db.DB) (*BookmarkStore, error) {
	coll, err := store.NewCollection[Bookmark](d, "bookmarks")
	if err != nil {
		return nil, err
	}
	return &BookmarkStore{coll: coll}, nil
}

// List returns all bookmarks for a user, newest first.
func (s *BookmarkStore) List(userID string) ([]Bookmark, error) {
	return s.coll.Where("user_id", "==", userID).OrderBy("created_at", store.Desc).All()
}

// Get returns a single bookmark by ID. Returns nil if not found.
func (s *BookmarkStore) Get(id string) (*Bookmark, error) {
	return s.coll.Get(id)
}

// Create adds a new bookmark.
func (s *BookmarkStore) Create(userID, url, title, tags string) (*Bookmark, error) {
	b := &Bookmark{
		ID:        uuid.New().String(),
		UserID:    userID,
		URL:       url,
		Title:     title,
		Tags:      tags,
		CreatedAt: time.Now().Format(time.RFC3339),
	}
	if err := s.coll.Create(b); err != nil {
		return nil, err
	}
	return b, nil
}

// Delete removes a bookmark by ID.
func (s *BookmarkStore) Delete(id string) error {
	return s.coll.Delete(id)
}
