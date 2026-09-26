package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

// testStore runs the same suite against any Store. Stage 4 will call it
// with the SQLite implementation too, which is the whole reason the
// interface exists.
func testStore(t *testing.T, newStore func(t *testing.T) Store) {
	t.Helper()

	ctx := context.Background()
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	future := now.Add(time.Hour)
	past := now.Add(-time.Hour)

	// Table-driven: each row is one scenario. Adding a case is adding a row,
	// not a new function. The closure gets a fresh store so cases can't
	// leak state into each other.
	tests := []struct {
		name string
		run  func(t *testing.T, s Store)
	}{
		{
			name: "put then get",
			run: func(t *testing.T, s Store) {
				want := Paste{ID: "a", Content: []byte("hello"), CreatedAt: now, ExpiresAt: future}
				if err := s.Put(ctx, want); err != nil {
					t.Fatalf("Put: %v", err)
				}
				got, err := s.Get(ctx, "a")
				if err != nil {
					t.Fatalf("Get: %v", err)
				}
				if got.ID != want.ID || string(got.Content) != string(want.Content) {
					t.Errorf("Get = %+v, want %+v", got, want)
				}
			},
		},
		{
			name: "get unknown id",
			run: func(t *testing.T, s Store) {
				_, err := s.Get(ctx, "nope")
				if !errors.Is(err, ErrNotFound) {
					t.Errorf("Get unknown: err = %v, want ErrNotFound", err)
				}
			},
		},
		{
			name: "put replaces existing",
			run: func(t *testing.T, s Store) {
				p := Paste{ID: "a", Content: []byte("v1"), CreatedAt: now, ExpiresAt: future}
				_ = s.Put(ctx, p)
				p.Content = []byte("v2")
				if err := s.Put(ctx, p); err != nil {
					t.Fatalf("Put v2: %v", err)
				}
				got, _ := s.Get(ctx, "a")
				if string(got.Content) != "v2" {
					t.Errorf("Content = %q, want v2", got.Content)
				}
			},
		},
		{
			name: "get does not alias stored content",
			run: func(t *testing.T, s Store) {
				// The slice footgun from Phase 1: if Put keeps the caller's
				// slice and Get hands the same one back, mutating the
				// returned slice corrupts the store.
				buf := []byte("original")
				_ = s.Put(ctx, Paste{ID: "a", Content: buf, ExpiresAt: future})
				buf[0] = 'X'
				got, _ := s.Get(ctx, "a")
				if string(got.Content) != "original" {
					t.Errorf("stored content changed after caller mutated its slice: %q", got.Content)
				}
				got.Content[0] = 'Y'
				again, _ := s.Get(ctx, "a")
				if string(again.Content) != "original" {
					t.Errorf("stored content changed after mutating Get result: %q", again.Content)
				}
			},
		},
		{
			name: "expired paste is not found",
			run: func(t *testing.T, s Store) {
				_ = s.Put(ctx, Paste{ID: "old", Content: []byte("x"), ExpiresAt: past})
				_, err := s.Get(ctx, "old")
				if !errors.Is(err, ErrNotFound) {
					t.Errorf("Get expired: err = %v, want ErrNotFound", err)
				}
			},
		},
		{
			name: "delete then get",
			run: func(t *testing.T, s Store) {
				_ = s.Put(ctx, Paste{ID: "a", Content: []byte("x"), ExpiresAt: future})
				if err := s.Delete(ctx, "a"); err != nil {
					t.Fatalf("Delete: %v", err)
				}
				if _, err := s.Get(ctx, "a"); !errors.Is(err, ErrNotFound) {
					t.Errorf("Get after Delete: err = %v, want ErrNotFound", err)
				}
			},
		},
		{
			name: "delete unknown id",
			run: func(t *testing.T, s Store) {
				if err := s.Delete(ctx, "nope"); !errors.Is(err, ErrNotFound) {
					t.Errorf("Delete unknown: err = %v, want ErrNotFound", err)
				}
			},
		},
		{
			name: "expire removes only expired",
			run: func(t *testing.T, s Store) {
				_ = s.Put(ctx, Paste{ID: "old1", Content: []byte("x"), ExpiresAt: past})
				_ = s.Put(ctx, Paste{ID: "old2", Content: []byte("x"), ExpiresAt: past})
				_ = s.Put(ctx, Paste{ID: "new", Content: []byte("x"), ExpiresAt: future})
				n, err := s.Expire(ctx, now)
				if err != nil {
					t.Fatalf("Expire: %v", err)
				}
				if n != 2 {
					t.Errorf("Expire removed %d, want 2", n)
				}
				if _, err := s.Get(ctx, "new"); err != nil {
					t.Errorf("Get new after Expire: %v", err)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.run(t, newStore(t))
		})
	}
}

func TestMemory(t *testing.T) {
	testStore(t, func(t *testing.T) Store {
		m := NewMemory()
		m.now = func() time.Time { return time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC) }
		return m
	})
}
