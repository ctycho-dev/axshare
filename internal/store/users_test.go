package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

var testNow = time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)

// newTestSQLite opens a fresh database in a temp directory with a fixed clock.
func newTestSQLite(t *testing.T) *SQLite {
	t.Helper()
	s, err := OpenSQLite(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	s.now = func() time.Time { return testNow }
	return s
}

func TestUpsertUser(t *testing.T) {
	ctx := context.Background()
	s := newTestSQLite(t)

	first, err := s.UpsertUser(ctx, "github", "42", User{Name: "Ada", Email: "ada@example.com"})
	if err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	if first.ID == 0 {
		t.Fatal("new user has id 0")
	}

	// The same login again is the same user, with the profile refreshed.
	again, err := s.UpsertUser(ctx, "github", "42", User{
		Name:      "Ada L.",
		Email:     "ada@example.com",
		AvatarURL: "https://example.com/a.png",
	})
	if err != nil {
		t.Fatalf("UpsertUser again: %v", err)
	}
	if again.ID != first.ID {
		t.Errorf("second sign-in got user %d, want %d", again.ID, first.ID)
	}
	if again.Name != "Ada L." || again.AvatarURL != "https://example.com/a.png" {
		t.Errorf("profile not refreshed: %+v", again)
	}

	// A different login is a different user.
	other, err := s.UpsertUser(ctx, "github", "43", User{Name: "Bob"})
	if err != nil {
		t.Fatalf("UpsertUser other: %v", err)
	}
	if other.ID == first.ID {
		t.Errorf("different login got the same user id %d", other.ID)
	}
}

func TestSessions(t *testing.T) {
	ctx := context.Background()
	s := newTestSQLite(t)

	u, err := s.UpsertUser(ctx, "github", "42", User{Name: "Ada"})
	if err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}

	const token = "raw-cookie-token"
	if err := s.CreateSession(ctx, token, u.ID, testNow.Add(time.Hour)); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	got, err := s.UserBySession(ctx, token)
	if err != nil {
		t.Fatalf("UserBySession: %v", err)
	}
	if got.ID != u.ID {
		t.Errorf("UserBySession = user %d, want %d", got.ID, u.ID)
	}

	// The raw token must not be in the database.
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions WHERE id = ?`, token).Scan(&n); err != nil {
		t.Fatalf("count raw token: %v", err)
	}
	if n != 0 {
		t.Error("the raw session token is stored in the database")
	}

	// Unknown token.
	if _, err := s.UserBySession(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown token: err = %v, want ErrNotFound", err)
	}

	// Expired session is not found, and Expire sweeps it.
	if err := s.CreateSession(ctx, "old", u.ID, testNow.Add(-time.Hour)); err != nil {
		t.Fatalf("CreateSession old: %v", err)
	}
	if _, err := s.UserBySession(ctx, "old"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expired session: err = %v, want ErrNotFound", err)
	}
	if _, err := s.Expire(ctx, testNow); err != nil {
		t.Fatalf("Expire: %v", err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions`).Scan(&n); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if n != 1 {
		t.Errorf("sessions after Expire = %d, want 1", n)
	}

	// Signing out kills the session; doing it twice is harmless.
	if err := s.DeleteSession(ctx, token); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if _, err := s.UserBySession(ctx, token); !errors.Is(err, ErrNotFound) {
		t.Errorf("after sign-out: err = %v, want ErrNotFound", err)
	}
	if err := s.DeleteSession(ctx, token); err != nil {
		t.Errorf("second DeleteSession: %v", err)
	}
}
