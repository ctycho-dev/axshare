package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// SQLite is a Store backed by a single file. *sql.DB is a connection pool,
// not a connection, and is safe for concurrent use, so unlike Memory there
// is no mutex here: SQLite itself serializes writers.
type SQLite struct {
	db  *sql.DB
	now func() time.Time
}

// OpenSQLite opens (creating if needed) the database at path and applies
// any pending migrations. Use ":memory:" for a throwaway database in tests.
func OpenSQLite(path string) (*SQLite, error) {
	// WAL lets readers proceed while a write is in progress; busy_timeout
	// makes a second writer wait instead of failing immediately with
	// "database is locked".
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)", path)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// sql.Open is lazy; Ping forces a real connection so a bad path fails
	// here, at startup, not on the first request.
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	if err := migrate(context.Background(), db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return &SQLite{db: db, now: time.Now}, nil
}

// Close releases the pool. Call it with defer in run().
func (s *SQLite) Close() error {
	return s.db.Close()
}

// Put is an upsert: insert, or on a duplicate id replace content and
// expiry. ext, owner_id and created_at are deliberately absent from the
// UPDATE list, so an existing room keeps them.
func (s *SQLite) Put(ctx context.Context, p Room) error {
	ext := p.Ext
	if ext == "" {
		ext = DefaultExt
	}
	// Valid=false makes the driver write NULL, which is how an anonymous
	// room is stored.
	owner := sql.NullInt64{Int64: p.OwnerID, Valid: p.OwnerID != 0}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO rooms (id, content, ext, owner_id, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			content    = excluded.content,
			expires_at = excluded.expires_at`,
		p.ID, p.Content, ext, owner, p.CreatedAt.Unix(), p.ExpiresAt.Unix(),
	)
	if err != nil {
		return fmt.Errorf("put %s: %w", p.ID, err)
	}
	return nil
}

func (s *SQLite) Get(ctx context.Context, id string) (Room, error) {
	var p Room
	var owner sql.NullInt64
	var created, expires int64

	row := s.db.QueryRowContext(ctx, `SELECT id, content, ext, owner_id, created_at, expires_at FROM rooms WHERE id = ?`, id)
	err := row.Scan(&p.ID, &p.Content, &p.Ext, &owner, &created, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return Room{}, ErrNotFound
	}
	if err != nil {
		return Room{}, fmt.Errorf("get %s: %w", id, err)
	}

	p.OwnerID = owner.Int64 // 0 when the column is NULL
	p.CreatedAt = time.Unix(created, 0)
	p.ExpiresAt = time.Unix(expires, 0)
	if p.ExpiresAt.Before(s.now()) {
		return Room{}, ErrNotFound
	}
	return p, nil
}

func (s *SQLite) RoomsByOwner(ctx context.Context, ownerID int64) ([]RoomInfo, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, ext, LENGTH(content), expires_at
		FROM rooms
		WHERE owner_id = ? AND expires_at >= ?
		ORDER BY expires_at DESC`,
		ownerID, s.now().Unix(),
	)
	if err != nil {
		return nil, fmt.Errorf("rooms by owner: %w", err)
	}
	defer rows.Close()

	var out []RoomInfo
	for rows.Next() {
		var r RoomInfo
		var expires int64
		if err := rows.Scan(&r.ID, &r.Ext, &r.Bytes, &expires); err != nil {
			return nil, fmt.Errorf("rooms by owner: scan: %w", err)
		}
		r.ExpiresAt = time.Unix(expires, 0)
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rooms by owner: %w", err)
	}
	return out, nil
}

func (s *SQLite) SetExt(ctx context.Context, id, ext string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE rooms SET ext = ? WHERE id = ? AND expires_at >= ?`,
		ext, id, s.now().Unix(),
	)
	if err != nil {
		return fmt.Errorf("set ext %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("set ext %s: rows affected: %w", id, err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLite) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM rooms WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete %s: rows affected: %w", id, err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Expire deletes expired rooms and sessions. It returns the number of rooms
// removed.
func (s *SQLite) Expire(ctx context.Context, now time.Time) (int, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM rooms WHERE expires_at < ?`, now.Unix())
	if err != nil {
		return 0, fmt.Errorf("expire: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("expire: rows affected: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at < ?`, now.Unix()); err != nil {
		return int(n), fmt.Errorf("expire sessions: %w", err)
	}
	return int(n), nil
}
