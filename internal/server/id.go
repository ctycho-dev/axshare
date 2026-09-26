package server

import (
	"crypto/rand"
	"encoding/base64"
)

// newID returns 128 bits of randomness as a 22-char URL-safe string.
// crypto/rand, not math/rand: paste IDs are the only access control we
// have, so they must be unguessable. RawURLEncoding drops the "=" padding
// and uses "-" and "_" instead of "+" and "/", so the ID is safe in a path.
func newID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
