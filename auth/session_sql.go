package auth

import (
	"database/sql"
	"fmt"
	"time"

	appdb "github.com/michaelwinser/appbase/db"
)

// sqlSessionBackend stores sessions in a SQL database (SQLite, Postgres).
type sqlSessionBackend struct {
	db *appdb.DB
}

func (b *sqlSessionBackend) Init() error {
	if err := b.db.Migrate(`
		CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			email TEXT NOT NULL,
			expires_at TEXT NOT NULL,
			created_at TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
		CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);
	`); err != nil {
		return err
	}
	// Add token columns for OAuth API access (safe: ignore "duplicate column" errors
	// from databases that already have them).
	for _, stmt := range []string{
		"ALTER TABLE sessions ADD COLUMN access_token TEXT DEFAULT ''",
		"ALTER TABLE sessions ADD COLUMN refresh_token TEXT DEFAULT ''",
		"ALTER TABLE sessions ADD COLUMN token_expiry TEXT DEFAULT ''",
	} {
		b.db.Exec(stmt) // ignore errors — column may already exist
	}
	return nil
}

func (b *sqlSessionBackend) Create(session *Session) error {
	_, err := b.db.Exec(
		`INSERT INTO sessions (id, user_id, email, expires_at, created_at) VALUES (?, ?, ?, ?, ?)`,
		session.ID, session.UserID, session.Email,
		session.ExpiresAt.Format(time.RFC3339),
		session.CreatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("creating session: %w", err)
	}
	return nil
}

func (b *sqlSessionBackend) Get(id string) (*Session, error) {
	row := b.db.QueryRow(
		`SELECT id, user_id, email, expires_at, created_at, `+
			`COALESCE(access_token, ''), COALESCE(refresh_token, ''), COALESCE(token_expiry, '') `+
			`FROM sessions WHERE id = ?`, id,
	)
	var session Session
	var expiresAt, createdAt, tokenExpiry string
	err := row.Scan(&session.ID, &session.UserID, &session.Email, &expiresAt, &createdAt,
		&session.AccessToken, &session.RefreshToken, &tokenExpiry)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting session: %w", err)
	}
	var parseErr error
	session.ExpiresAt, parseErr = time.Parse(time.RFC3339, expiresAt)
	if parseErr != nil {
		return nil, fmt.Errorf("parsing session expiry %q: %w", expiresAt, parseErr)
	}
	session.CreatedAt, parseErr = time.Parse(time.RFC3339, createdAt)
	if parseErr != nil {
		return nil, fmt.Errorf("parsing session created_at %q: %w", createdAt, parseErr)
	}
	if tokenExpiry != "" {
		session.TokenExpiry, _ = time.Parse(time.RFC3339, tokenExpiry)
	}
	return &session, nil
}

func (b *sqlSessionBackend) UpdateTokens(sessionID, accessToken, refreshToken string, tokenExpiry time.Time) error {
	_, err := b.db.Exec(
		`UPDATE sessions SET access_token = ?, refresh_token = ?, token_expiry = ? WHERE id = ?`,
		accessToken, refreshToken, tokenExpiry.Format(time.RFC3339), sessionID,
	)
	if err != nil {
		return fmt.Errorf("updating tokens: %w", err)
	}
	return nil
}

func (b *sqlSessionBackend) Delete(id string) error {
	_, err := b.db.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	return err
}

func (b *sqlSessionBackend) DeleteExpired() error {
	_, err := b.db.Exec(`DELETE FROM sessions WHERE expires_at < ?`, time.Now().Format(time.RFC3339))
	return err
}

func (b *sqlSessionBackend) DeleteByUser(userID string) error {
	_, err := b.db.Exec(`DELETE FROM sessions WHERE user_id = ?`, userID)
	return err
}
