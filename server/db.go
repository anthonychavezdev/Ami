package main

import (
	"database/sql"

	_ "modernc.org/sqlite" // pure-Go SQLite driver (the only non-stdlib dependency)
)

const schema = `
CREATE TABLE IF NOT EXISTS users (
	id            INTEGER PRIMARY KEY,
	username      TEXT NOT NULL UNIQUE COLLATE NOCASE,
	display_name  TEXT NOT NULL,
	password_hash BLOB NOT NULL,
	salt          BLOB NOT NULL,
	created_at    TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
	token      TEXT PRIMARY KEY,
	user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	expires_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS posts (
	id         INTEGER PRIMARY KEY,
	author_id  INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	body       TEXT NOT NULL,
	created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS posts_author ON posts(author_id, id);

CREATE TABLE IF NOT EXISTS comments (
	id         INTEGER PRIMARY KEY,
	post_id    INTEGER NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
	author_id  INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	body       TEXT NOT NULL,
	created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS comments_post ON comments(post_id, id);

CREATE TABLE IF NOT EXISTS reactions (
	post_id INTEGER NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
	user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	emoji   TEXT NOT NULL,
	PRIMARY KEY (post_id, user_id)
);

-- One row per pair. from_id is whoever sent the request.
CREATE TABLE IF NOT EXISTS friendships (
	from_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	to_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	status     TEXT NOT NULL CHECK (status IN ('pending', 'accepted')),
	created_at TEXT NOT NULL,
	PRIMARY KEY (from_id, to_id)
);
CREATE INDEX IF NOT EXISTS friendships_to ON friendships(to_id);
`

func openDB(path string) (*sql.DB, error) {
	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// SQLite serializes writers anyway; a single connection avoids SQLITE_BUSY entirely.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}
