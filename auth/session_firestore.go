package auth

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	appdb "github.com/michaelwinser/appbase/db"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const firestoreSessionCollection = "sessions"

// firestoreSessionBackend stores sessions in Firestore.
type firestoreSessionBackend struct {
	db *appdb.DB
}

// firestoreSession is the Firestore document representation of a session.
type firestoreSession struct {
	UserID       string    `firestore:"user_id"`
	Email        string    `firestore:"email"`
	ExpiresAt    time.Time `firestore:"expires_at"`
	CreatedAt    time.Time `firestore:"created_at"`
	AccessToken  string    `firestore:"access_token,omitempty"`
	RefreshToken string    `firestore:"refresh_token,omitempty"`
	TokenExpiry  time.Time `firestore:"token_expiry,omitempty"`
}

func (b *firestoreSessionBackend) Init() error {
	// Firestore is schemaless — nothing to initialize
	return nil
}

func (b *firestoreSessionBackend) Create(session *Session) error {
	ctx := context.Background()
	doc := firestoreSession{
		UserID:    session.UserID,
		Email:     session.Email,
		ExpiresAt: session.ExpiresAt,
		CreatedAt: session.CreatedAt,
	}
	_, err := b.db.Firestore().Collection(firestoreSessionCollection).Doc(session.ID).Set(ctx, doc)
	if err != nil {
		return fmt.Errorf("creating session: %w", err)
	}
	return nil
}

func (b *firestoreSessionBackend) Get(id string) (*Session, error) {
	ctx := context.Background()
	doc, err := b.db.Firestore().Collection(firestoreSessionCollection).Doc(id).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("getting session: %w", err)
	}

	var fs firestoreSession
	if err := doc.DataTo(&fs); err != nil {
		return nil, fmt.Errorf("decoding session: %w", err)
	}

	return &Session{
		ID:           doc.Ref.ID,
		UserID:       fs.UserID,
		Email:        fs.Email,
		ExpiresAt:    fs.ExpiresAt,
		CreatedAt:    fs.CreatedAt,
		AccessToken:  fs.AccessToken,
		RefreshToken: fs.RefreshToken,
		TokenExpiry:  fs.TokenExpiry,
	}, nil
}

func (b *firestoreSessionBackend) UpdateTokens(sessionID, accessToken, refreshToken string, tokenExpiry time.Time) error {
	ctx := context.Background()
	_, err := b.db.Firestore().Collection(firestoreSessionCollection).Doc(sessionID).Update(ctx, []firestore.Update{
		{Path: "access_token", Value: accessToken},
		{Path: "refresh_token", Value: refreshToken},
		{Path: "token_expiry", Value: tokenExpiry},
	})
	if err != nil {
		return fmt.Errorf("updating tokens: %w", err)
	}
	return nil
}

func (b *firestoreSessionBackend) Delete(id string) error {
	ctx := context.Background()
	_, err := b.db.Firestore().Collection(firestoreSessionCollection).Doc(id).Delete(ctx)
	return err
}

func (b *firestoreSessionBackend) DeleteExpired() error {
	ctx := context.Background()
	col := b.db.Firestore().Collection(firestoreSessionCollection)
	iter := col.Where("expires_at", "<", time.Now()).Documents(ctx)
	defer iter.Stop()

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return fmt.Errorf("querying expired sessions: %w", err)
		}
		if _, err := doc.Ref.Delete(ctx); err != nil {
			return fmt.Errorf("deleting expired session: %w", err)
		}
	}
	return nil
}

func (b *firestoreSessionBackend) DeleteByUser(userID string) error {
	ctx := context.Background()
	col := b.db.Firestore().Collection(firestoreSessionCollection)
	iter := col.Where("user_id", "==", userID).Documents(ctx)
	defer iter.Stop()

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return fmt.Errorf("querying user sessions: %w", err)
		}
		if _, err := doc.Ref.Delete(ctx); err != nil {
			return fmt.Errorf("deleting user session: %w", err)
		}
	}
	return nil
}
