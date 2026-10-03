package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// hashToken turns a session cookie value into the id stored in the sessions
// table, so the database never holds a usable token.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

const userColumns = `id, name, email, avatar_url, created_at`

func scanUser(row *sql.Row) (User, error) {
	var u User
	var created int64
	if err := row.Scan(&u.ID, &u.Name, &u.Email, &u.AvatarURL, &created); err != nil {
		return User{}, err
	}
	u.CreatedAt = time.Unix(created, 0)
	return u, nil
}

func (s *SQLite) UpsertUser(ctx context.Context, provider, providerUserID string, u User) (User, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, fmt.Errorf("upsert user: begin: %w", err)
	}
	defer tx.Rollback()

	var id int64
	err = tx.QueryRowContext(ctx,
		`SELECT user_id FROM identities WHERE provider = ? AND provider_user_id = ?`,
		provider, providerUserID,
	).Scan(&id)

	if errors.Is(err, sql.ErrNoRows) {
		// First sign-in with this login: create the user and the identity.
		id, err = insertUser(ctx, tx, provider, providerUserID, u, s.now().Unix())
	} else if err == nil {
		// Known login: refresh the profile from the provider.
		_, err = tx.ExecContext(ctx,
			`UPDATE users SET name = ?, email = ?, avatar_url = ? WHERE id = ?`,
			u.Name, u.Email, u.AvatarURL, id,
		)
	}
	if err != nil {
		return User{}, fmt.Errorf("upsert user: %w", err)
	}

	out, err := scanUser(tx.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE id = ?`, id))
	if err != nil {
		return User{}, fmt.Errorf("upsert user: read back: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return User{}, fmt.Errorf("upsert user: commit: %w", err)
	}
	return out, nil
}

// insertUser writes the user row and its identity row inside tx and returns
// the new user id.
func insertUser(ctx context.Context, tx *sql.Tx, provider, providerUserID string, u User, now int64) (int64, error) {
	res, err := tx.ExecContext(ctx,
		`INSERT INTO users (name, email, avatar_url, created_at) VALUES (?, ?, ?, ?)`,
		u.Name, u.Email, u.AvatarURL, now,
	)
	if err != nil {
		return 0, fmt.Errorf("insert user: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("insert user: last insert id: %w", err)
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO identities (provider, provider_user_id, user_id, created_at) VALUES (?, ?, ?, ?)`,
		provider, providerUserID, id, now,
	)
	if err != nil {
		return 0, fmt.Errorf("insert identity: %w", err)
	}
	return id, nil
}

func (s *SQLite) CreateSession(ctx context.Context, token string, userID int64, expiresAt time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions (id, user_id, created_at, expires_at) VALUES (?, ?, ?, ?)`,
		hashToken(token), userID, s.now().Unix(), expiresAt.Unix(),
	)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

func (s *SQLite) UserBySession(ctx context.Context, token string) (User, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT u.id, u.name, u.email, u.avatar_url, u.created_at
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.id = ? AND s.expires_at >= ?`,
		hashToken(token), s.now().Unix(),
	)
	u, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("user by session: %w", err)
	}
	return u, nil
}

func (s *SQLite) DeleteSession(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, hashToken(token))
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}
