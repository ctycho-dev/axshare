package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
	// Blank import: we never call the package directly. Its init() registers
	// the driver with database/sql under the name "sqlite", and that side
	// effect is all we want. Without the underscore the compiler rejects an
	// unused import.
	_ "modernc.org/sqlite"
)

// schema runs on every Open. IF NOT EXISTS makes it idempotent, which is
// enough until there's a second version of the table; then it becomes a
// migration.
//
// Times are stored as Unix seconds (INTEGER) rather than TEXT: cheaper to
// compare in SQL and no timezone ambiguity.
const schema = `
CREATE TABLE IF NOT EXISTS pastes (
	id         TEXT    PRIMARY KEY,
	content    BLOB    NOT NULL,
	created_at INTEGER NOT NULL,
	expires_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS pastes_expires_at ON pastes (expires_at);
`

// SQLite is a Store backed by a single file. *sql.DB is a connection pool,
// not a connection, and is safe for concurrent use, so unlike Memory there
// is no mutex here: SQLite itself serializes writers.
type SQLite struct {
	db  *sql.DB
	now func() time.Time
}

// OpenSQLite opens (creating if needed) the database at path and applies
// the schema. Use ":memory:" for a throwaway database in tests.
func OpenSQLite(path string) (*SQLite, error) {
	// WAL lets readers proceed while a write is in progress; busy_timeout
	// makes a second writer wait instead of failing immediately with
	// "database is locked".
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", path)

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
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}

	return &SQLite{db: db, now: time.Now}, nil
}

// Close releases the pool. Call it with defer in run().
func (s *SQLite) Close() error {
	return s.db.Close()
}

// Put is an upsert: insert, or on a duplicate id replace the row.
// The ? placeholders are the only correct way to pass values; never
// fmt.Sprintf them into the SQL string.
func (s *SQLite) Put(ctx context.Context, p Paste) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO pastes (id, content, created_at, expires_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			content    = excluded.content,
			created_at = excluded.created_at,
			expires_at = excluded.expires_at`,
		p.ID, p.Content, p.CreatedAt.Unix(), p.ExpiresAt.Unix(),
	)
	if err != nil {
		return fmt.Errorf("put %s: %w", p.ID, err)
	}
	return nil
}

func (s *SQLite) Get(ctx context.Context, id string) (Paste, error) {
	var p Paste
	var created, expires int64

	row := s.db.QueryRowContext(ctx, `SELECT id, content, created_at, expires_at FROM pastes WHERE id = ?`, id)
	err := row.Scan(&p.ID, &p.Content, &created, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return Paste{}, ErrNotFound
	}
	if err != nil {
		return Paste{}, fmt.Errorf("get %s: %w", id, err)
	}

	p.CreatedAt = time.Unix(created, 0)
	p.ExpiresAt = time.Unix(expires, 0)
	if p.ExpiresAt.Before(s.now()) {
		return Paste{}, ErrNotFound
	}
	return p, nil
}

func (s *SQLite) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM pastes WHERE id = ?`, id)
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

func (s *SQLite) Expire(ctx context.Context, now time.Time) (int, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM pastes WHERE expires_at < ?`, now.Unix())
	if err != nil {
		return 0, fmt.Errorf("expire: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("expire: rows affected: %w", err)
	}
	return int(n), nil
}
