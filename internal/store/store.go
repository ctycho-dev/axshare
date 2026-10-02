package store

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("store: room not found")

// DefaultExt is the file type of a room that never had one set.
const DefaultExt = "plain"

type Room struct {
	ID        string
	Content   []byte
	Ext       string
	CreatedAt time.Time
	ExpiresAt time.Time
}

type Store interface {
	Put(ctx context.Context, p Room) error

	Get(ctx context.Context, id string) (Room, error)

	SetExt(ctx context.Context, id, ext string) error

	Delete(ctx context.Context, id string) error

	Expire(ctx context.Context, now time.Time) (int, error)
}

func ValidExt(ext string) bool {
	if len(ext) == 0 || len(ext) > 16 {
		return false
	}
	for _, c := range ext {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}
