package db

import (
	"context"
	"fmt"
	"log"
	"os"

	"cloud.google.com/go/firestore"
)

func newFirestore() (*DB, error) {
	project := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if project == "" {
		return nil, fmt.Errorf("GOOGLE_CLOUD_PROJECT is required for Firestore backend")
	}

	ctx := context.Background()
	client, err := firestore.NewClient(ctx, project)
	if err != nil {
		return nil, fmt.Errorf("creating Firestore client: %w", err)
	}

	log.Printf("Using Firestore store (project: %s)", project)
	return &DB{firestore: client, storeType: "firestore"}, nil
}
