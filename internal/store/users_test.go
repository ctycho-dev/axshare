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

func TestRoomOwnership(t *testing.T) {
	ctx := context.Background()
	s := newTestSQLite(t)

	u, err := s.UpsertUser(ctx, "github", "42", User{Name: "Ada"})
	if err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	other, err := s.UpsertUser(ctx, "github", "43", User{Name: "Bob"})
	if err != nil {
		t.Fatalf("UpsertUser other: %v", err)
	}

	soon := testNow.Add(time.Hour)
	later := testNow.Add(2 * time.Hour)
	past := testNow.Add(-time.Hour)

	rooms := []Room{
		{ID: "mine-old", Content: []byte("abc"), OwnerID: u.ID, ExpiresAt: soon},
		{ID: "mine-new", Content: []byte("hello"), Ext: "go", OwnerID: u.ID, ExpiresAt: later},
		{ID: "mine-expired", Content: []byte("x"), OwnerID: u.ID, ExpiresAt: past},
		{ID: "theirs", Content: []byte("x"), OwnerID: other.ID, ExpiresAt: soon},
		{ID: "anon", Content: []byte("x"), ExpiresAt: soon},
	}
	for _, r := range rooms {
		if err := s.Put(ctx, r); err != nil {
			t.Fatalf("Put %s: %v", r.ID, err)
		}
	}

	// An edit is a Put with no owner. It must not strip ownership.
	if err := s.Put(ctx, Room{ID: "mine-old", Content: []byte("abcd"), ExpiresAt: soon}); err != nil {
		t.Fatalf("Put edit: %v", err)
	}
	got, err := s.Get(ctx, "mine-old")
	if err != nil {
		t.Fatalf("Get mine-old: %v", err)
	}
	if got.OwnerID != u.ID {
		t.Errorf("owner after edit = %d, want %d", got.OwnerID, u.ID)
	}

	anon, err := s.Get(ctx, "anon")
	if err != nil {
		t.Fatalf("Get anon: %v", err)
	}
	if anon.OwnerID != 0 {
		t.Errorf("anonymous room owner = %d, want 0", anon.OwnerID)
	}

	// The listing has only this user's live rooms, most recently edited first.
	list, err := s.RoomsByOwner(ctx, u.ID)
	if err != nil {
		t.Fatalf("RoomsByOwner: %v", err)
	}
	if len(list) != 2 || list[0].ID != "mine-new" || list[1].ID != "mine-old" {
		t.Fatalf("RoomsByOwner = %+v, want mine-new then mine-old", list)
	}
	if list[0].Ext != "go" || list[0].Bytes != 5 {
		t.Errorf("mine-new info = %+v, want ext go and 5 bytes", list[0])
	}
}
