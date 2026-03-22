package db

import (
	"context"
	"fmt"
	"log"

	"cloud.google.com/go/firestore"
)

func newFirestoreWithProject(project string) (*DB, error) {
	if project == "" {
		return nil, fmt.Errorf("GCP project is required for Firestore backend (set GCPProject in DBConfig or GOOGLE_CLOUD_PROJECT env var)")
	}

	ctx := context.Background()
	client, err := firestore.NewClient(ctx, project)
	if err != nil {
		return nil, fmt.Errorf("creating Firestore client: %w", err)
	}

	log.Printf("Using Firestore store (project: %s)", project)
	return &DB{firestore: client, storeType: "firestore"}, nil
}
