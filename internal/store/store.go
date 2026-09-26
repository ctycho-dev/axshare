package store

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("store: paste not found")

type Paste struct {
	ID        string
	Content   []byte
	CreatedAt time.Time
	ExpiresAt time.Time
}

type Store interface {
	Put(ctx context.Context, p Paste) error

	Get(ctx context.Context, id string) (Paste, error)

	Delete(ctx context.Context, id string) error

	Expire(ctx context.Context, now time.Time) (int, error)
}
