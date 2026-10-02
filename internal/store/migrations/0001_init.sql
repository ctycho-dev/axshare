CREATE TABLE IF NOT EXISTS rooms (
	id         TEXT    PRIMARY KEY,
	content    BLOB    NOT NULL,
	created_at INTEGER NOT NULL,
	expires_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS rooms_expires_at ON rooms (expires_at);