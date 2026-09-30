package store

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("store: room not found")

type Room struct {
	ID        string
	Content   []byte
	CreatedAt time.Time
	ExpiresAt time.Time
}

type Store interface {
	Put(ctx context.Context, p Room) error

	Get(ctx context.Context, id string) (Room, error)

	Delete(ctx context.Context, id string) error

	Expire(ctx context.Context, now time.Time) (int, error)
}
