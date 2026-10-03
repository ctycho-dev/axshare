package store

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("store: not found")

// DefaultExt is the file type of a room that never had one set.
const DefaultExt = "plain"

type Room struct {
	ID        string
	Content   []byte
	Ext       string
	CreatedAt time.Time
	ExpiresAt time.Time
}

type User struct {
	ID        int64
	Name      string
	Email     string
	AvatarURL string
	CreatedAt time.Time
}

type Store interface {
	// Put creates the room or, if the id exists, replaces its content and
	// timestamps. Ext is used on create only (empty means DefaultExt); an
	// existing room keeps the Ext it has. Use SetExt to change it.
	Put(ctx context.Context, p Room) error

	Get(ctx context.Context, id string) (Room, error)

	// SetExt changes the room's file type. It returns ErrNotFound if the
	// room does not exist or has expired.
	SetExt(ctx context.Context, id, ext string) error

	Delete(ctx context.Context, id string) error

	Expire(ctx context.Context, now time.Time) (int, error)

	// UpsertUser finds the user behind a provider login, creating the user
	// on first sign-in and refreshing name, email and avatar on later ones.
	UpsertUser(ctx context.Context, provider, providerUserID string, u User) (User, error)

	// CreateSession stores a session for the raw cookie token. Only a hash
	// of the token is written to the database.
	CreateSession(ctx context.Context, token string, userID int64, expiresAt time.Time) error

	// UserBySession returns the user for a raw cookie token, or ErrNotFound
	// if the session does not exist or has expired.
	UserBySession(ctx context.Context, token string) (User, error)

	// DeleteSession removes a session. Deleting one that does not exist is
	// not an error, so signing out twice is harmless.
	DeleteSession(ctx context.Context, token string) error
}

// ValidExt reports whether ext is acceptable as a room file type: 1 to 16
// lowercase letters or digits. The list of languages lives in the frontend;
// the server only refuses garbage.
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
