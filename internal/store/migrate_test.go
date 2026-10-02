package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

// embedded returns the embedded migrations, failing the test if there are
// none (which would mean the //go:embed directive is broken).
func embedded(t *testing.T) []migration {
	t.Helper()
	ms, err := loadMigrations()
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	if len(ms) == 0 {
		t.Fatal("no embedded migrations: check the //go:embed directive")
	}
	return ms
}

func TestMigrateFreshDB(t *testing.T) {
	ctx := context.Background()
	ms := embedded(t)

	s, err := OpenSQLite(filepath.Join(t.TempDir(), "fresh.db"))
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer s.Close()

	got, err := currentVersion(ctx, s.db)
	if err != nil {
		t.Fatalf("currentVersion: %v", err)
	}
	if want := ms[len(ms)-1].version; got != want {
		t.Fatalf("version = %d, want %d", got, want)
	}
}

func TestMigrateTwiceIsNoOp(t *testing.T) {
	ctx := context.Background()
	ms := embedded(t)

	s, err := OpenSQLite(filepath.Join(t.TempDir(), "twice.db"))
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer s.Close()

	if err := migrate(ctx, s.db); err != nil {
		t.Fatalf("second migrate: %v", err)
	}

	var rows int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_version`).Scan(&rows); err != nil {
		t.Fatalf("count schema_version: %v", err)
	}
	if rows != len(ms) {
		t.Fatalf("schema_version has %d rows, want %d", rows, len(ms))
	}
}

// TestMigrateLegacyDB simulates the live database: a rooms table created by
// the old inline schema, with data, and no schema_version table.
func TestMigrateLegacyDB(t *testing.T) {
	ctx := context.Background()
	ms := embedded(t)
	path := filepath.Join(t.TempDir(), "legacy.db")

	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	_, err = raw.ExecContext(ctx, `
		CREATE TABLE rooms (
			id         TEXT    PRIMARY KEY,
			content    BLOB    NOT NULL,
			created_at INTEGER NOT NULL,
			expires_at INTEGER NOT NULL
		);
		CREATE INDEX rooms_expires_at ON rooms (expires_at);`)
	if err != nil {
		t.Fatalf("create legacy schema: %v", err)
	}
	now := time.Now()
	_, err = raw.ExecContext(ctx,
		`INSERT INTO rooms (id, content, created_at, expires_at) VALUES (?, ?, ?, ?)`,
		"legacy", []byte("hello"), now.Unix(), now.Add(time.Hour).Unix(),
	)
	if err != nil {
		t.Fatalf("insert legacy room: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close raw: %v", err)
	}

	s, err := OpenSQLite(path)
	if err != nil {
		t.Fatalf("OpenSQLite on legacy db: %v", err)
	}
	defer s.Close()

	room, err := s.Get(ctx, "legacy")
	if err != nil {
		t.Fatalf("Get after migrate: %v", err)
	}
	if got := string(room.Content); got != "hello" {
		t.Fatalf("content = %q, want %q", got, "hello")
	}

	got, err := currentVersion(ctx, s.db)
	if err != nil {
		t.Fatalf("currentVersion: %v", err)
	}
	if want := ms[len(ms)-1].version; got != want {
		t.Fatalf("version = %d, want %d", got, want)
	}
}
