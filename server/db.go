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
	password_iterations INTEGER NOT NULL DEFAULT 210000,
	invisible     INTEGER NOT NULL DEFAULT 0,
	created_at    TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
	token      TEXT PRIMARY KEY,
	user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	expires_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS groups (
	id          INTEGER PRIMARY KEY,
	name        TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	owner_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	created_at  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS group_members (
	group_id  INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
	user_id   INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	role      TEXT NOT NULL CHECK (role IN ('admin', 'member')),
	joined_at TEXT NOT NULL,
	PRIMARY KEY (group_id, user_id)
);
CREATE INDEX IF NOT EXISTS group_members_user ON group_members(user_id);

CREATE TABLE IF NOT EXISTS posts (
	id         INTEGER PRIMARY KEY,
	author_id  INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	group_id   INTEGER REFERENCES groups(id) ON DELETE CASCADE,
	body       TEXT NOT NULL,
	attachments TEXT NOT NULL DEFAULT '[]',
	edited_at   TEXT,
	created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS posts_author ON posts(author_id, id);
CREATE INDEX IF NOT EXISTS posts_group ON posts(group_id, id);

CREATE TABLE IF NOT EXISTS comments (
	id         INTEGER PRIMARY KEY,
	post_id    INTEGER NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
	author_id  INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	parent_id  INTEGER REFERENCES comments(id),
	body       TEXT NOT NULL,
	created_at TEXT NOT NULL,
	edited_at  TEXT,
	deleted_at TEXT
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

CREATE TABLE IF NOT EXISTS messages (
	id           INTEGER PRIMARY KEY,
	sender_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	recipient_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	body         TEXT NOT NULL,
	attachments  TEXT NOT NULL DEFAULT '[]',
	edited_at   TEXT,
	created_at   TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS messages_sender ON messages(sender_id, recipient_id, id);
CREATE INDEX IF NOT EXISTS messages_recipient ON messages(recipient_id, sender_id, id);

CREATE TABLE IF NOT EXISTS conversation_reads (
	user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	peer_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	last_read_id INTEGER NOT NULL,
	PRIMARY KEY (user_id, peer_id)
);

CREATE TABLE IF NOT EXISTS push_tokens (
	token      TEXT PRIMARY KEY,
	user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS push_tokens_user ON push_tokens(user_id);
`

func openDB(path string) (*sql.DB, error) {
	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// SQLite serializes writers anyway; a single connection avoids SQLITE_BUSY entirely.
	db.SetMaxOpenConns(1)
	var hadReads int
	_ = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'conversation_reads'`).Scan(&hadReads)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	// Columns added after the first release; CREATE TABLE IF NOT EXISTS won't add them.
	for _, table := range []string{"posts", "messages"} {
		for _, col := range [][2]string{{"attachments", "TEXT NOT NULL DEFAULT '[]'"}, {"edited_at", "TEXT"}} {
			if err := addColumn(db, table, col[0], col[1]); err != nil {
				db.Close()
				return nil, err
			}
		}
	}
	if err := addColumn(db, "users", "password_iterations", "INTEGER NOT NULL DEFAULT 210000"); err != nil {
		db.Close()
		return nil, err
	}
	if err := addColumn(db, "users", "invisible", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		db.Close()
		return nil, err
	}
	for _, col := range [][2]string{{"parent_id", "INTEGER"}, {"edited_at", "TEXT"}, {"deleted_at", "TEXT"}} {
		if err := addColumn(db, "comments", col[0], col[1]); err != nil {
			db.Close()
			return nil, err
		}
	}
	// Only after the column exists on older databases.
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS comments_parent ON comments(parent_id)`); err != nil {
		db.Close()
		return nil, err
	}
	if hadReads == 0 {
		// Chats from before unread tracking start out read.
		if _, err := db.Exec(`INSERT OR IGNORE INTO conversation_reads(user_id, peer_id, last_read_id)
			SELECT recipient_id, sender_id, MAX(id) FROM messages GROUP BY recipient_id, sender_id`); err != nil {
			db.Close()
			return nil, err
		}
	}
	return db, nil
}

func addColumn(db *sql.DB, table, column, definition string) error {
	rows, err := db.Query(`SELECT 1 FROM pragma_table_info(?) WHERE name = ?`, table, column)
	if err != nil {
		return err
	}
	exists := rows.Next()
	rows.Close() // the single connection must be free before the ALTER below
	if exists {
		return nil
	}
	_, err = db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + column + ` ` + definition)
	return err
}
