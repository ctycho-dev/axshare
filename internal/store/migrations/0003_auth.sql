CREATE TABLE users (
	id         INTEGER PRIMARY KEY,
	name       TEXT    NOT NULL,
	email      TEXT    NOT NULL DEFAULT '',
	avatar_url TEXT    NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL
);

-- One row per way a user can sign in. Someone who signs in with GitHub and
-- later with Google can have two rows pointing at the same user.
CREATE TABLE identities (
	provider         TEXT    NOT NULL,
	provider_user_id TEXT    NOT NULL,
	user_id          INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
	created_at       INTEGER NOT NULL,
	PRIMARY KEY (provider, provider_user_id)
);
CREATE INDEX identities_user_id ON identities (user_id);

-- id is the SHA-256 of the cookie value, so a leaked database does not
-- contain usable session tokens.
CREATE TABLE sessions (
	id         TEXT    PRIMARY KEY,
	user_id    INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
	created_at INTEGER NOT NULL,
	expires_at INTEGER NOT NULL
);
CREATE INDEX sessions_expires_at ON sessions (expires_at);

ALTER TABLE rooms ADD COLUMN owner_id INTEGER REFERENCES users (id) ON DELETE SET NULL;
CREATE INDEX rooms_owner_id ON rooms (owner_id);