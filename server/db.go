package main

import (
	"database/sql"
	"fmt"
	"log"

	_ "modernc.org/sqlite" // pure-Go SQLite driver (the only non-stdlib dependency)
)

// schema is what a new database gets: every table and index of the current
// version. An existing database is brought here by the migrations below.
const schema = `
CREATE TABLE IF NOT EXISTS users (
	id            INTEGER PRIMARY KEY,
	username      TEXT NOT NULL UNIQUE COLLATE NOCASE,
	display_name  TEXT NOT NULL,
	password_hash BLOB NOT NULL,
	salt          BLOB NOT NULL,
	password_iterations INTEGER NOT NULL DEFAULT 210000, -- the count of the first release; login upgrades it (sessions.go)
	invisible     INTEGER NOT NULL DEFAULT 0,
	avatar_url    TEXT NOT NULL DEFAULT '',
	bio           TEXT NOT NULL DEFAULT '',
	pronouns      TEXT NOT NULL DEFAULT '',
	avatar_crop   TEXT NOT NULL DEFAULT '',
	must_change_password INTEGER NOT NULL DEFAULT 0, -- set by -reset-password; cleared by PUT /api/me/password
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
	private     INTEGER NOT NULL DEFAULT 0, -- invite only: hidden from discovery, members are added by the admin
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

-- A direct message has recipient_id, a group chat message has group_id.
CREATE TABLE IF NOT EXISTS messages (
	id           INTEGER PRIMARY KEY,
	sender_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	recipient_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
	group_id     INTEGER REFERENCES groups(id) ON DELETE CASCADE,
	body         TEXT NOT NULL,
	attachments  TEXT NOT NULL DEFAULT '[]',
	edited_at    TEXT,
	created_at   TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS messages_sender ON messages(sender_id, recipient_id, id);
CREATE INDEX IF NOT EXISTS messages_recipient ON messages(recipient_id, sender_id, id);

CREATE TABLE IF NOT EXISTS group_reads (
	user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	group_id     INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
	last_read_id INTEGER NOT NULL,
	PRIMARY KEY (user_id, group_id)
);

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

// lateIndexes are on columns that older databases gain in migration 1, so
// they are created after those columns exist.
const lateIndexes = `
CREATE INDEX IF NOT EXISTS comments_parent ON comments(parent_id);
CREATE INDEX IF NOT EXISTS messages_group ON messages(group_id, id);
`

// A database records its schema version in PRAGMA user_version. A new
// database gets `schema` and the latest version at once; an existing one
// runs every migration past its version, in order, each in a transaction.
//
// To change the schema: edit `schema` (what new databases get) and append a
// migration that brings an existing database to the same place. The version
// number is the migration's position, so nothing else needs bumping.
var migrations = []migration{
	{"everything up to group chats", upgradeUnversioned},
	{"password resets", func(tx *sql.Tx) error {
		return addColumn(tx, "users", "must_change_password", "INTEGER NOT NULL DEFAULT 0")
	}},
}

type migration struct {
	name string
	run  func(tx *sql.Tx) error
}

func openDB(path string) (*sql.DB, error) {
	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// SQLite serializes writers anyway; a single connection avoids SQLITE_BUSY entirely.
	db.SetMaxOpenConns(1)
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// migrate brings a database of any earlier version up to date.
func migrate(db *sql.DB) error {
	version, err := userVersion(db)
	if err != nil {
		return err
	}
	if version == 0 && !tableExists(db, "users") { // a new database
		if err := exec(db, schema+lateIndexes); err != nil {
			return err
		}
		return setUserVersion(db, len(migrations))
	}
	for ; version < len(migrations); version++ {
		m := migrations[version]
		err := inTx(db, func(tx *sql.Tx) error {
			if err := m.run(tx); err != nil {
				return err
			}
			return setUserVersion(tx, version+1)
		})
		if err != nil {
			return fmt.Errorf("migration %d (%s): %w", version+1, m.name, err)
		}
		log.Printf("db: migrated to version %d (%s)", version+1, m.name)
	}
	return nil
}

// dbtx is what *sql.DB and *sql.Tx have in common.
type dbtx interface {
	Exec(query string, args ...any) (sql.Result, error)
	QueryRow(query string, args ...any) *sql.Row
}

func exec(db dbtx, statements string) error {
	_, err := db.Exec(statements)
	return err
}

func userVersion(db dbtx) (int, error) {
	var v int
	err := db.QueryRow(`PRAGMA user_version`).Scan(&v)
	return v, err
}

func setUserVersion(db dbtx, v int) error {
	return exec(db, fmt.Sprintf(`PRAGMA user_version = %d`, v))
}

func tableExists(db dbtx, name string) bool {
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&n)
	return n > 0
}

func columnExists(db dbtx, table, column string) bool {
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, table, column).Scan(&n)
	return n > 0
}

// ---- migration 1

// Columns added between the first release and group chats; CREATE TABLE IF
// NOT EXISTS does not add them to a table that already exists.
var addedColumns = []struct{ table, column, definition string }{
	{"posts", "attachments", "TEXT NOT NULL DEFAULT '[]'"},
	{"posts", "edited_at", "TEXT"},
	{"messages", "attachments", "TEXT NOT NULL DEFAULT '[]'"},
	{"messages", "edited_at", "TEXT"},
	{"users", "password_iterations", "INTEGER NOT NULL DEFAULT 210000"},
	{"users", "invisible", "INTEGER NOT NULL DEFAULT 0"},
	{"users", "avatar_url", "TEXT NOT NULL DEFAULT ''"},
	{"users", "bio", "TEXT NOT NULL DEFAULT ''"},
	{"users", "pronouns", "TEXT NOT NULL DEFAULT ''"},
	{"users", "avatar_crop", "TEXT NOT NULL DEFAULT ''"},
	{"comments", "parent_id", "INTEGER"},
	{"comments", "edited_at", "TEXT"},
	{"comments", "deleted_at", "TEXT"},
	{"groups", "private", "INTEGER NOT NULL DEFAULT 0"},
}

// upgradeUnversioned brings a database from before versioning (any release up
// to group chats) to version 1. Those databases were upgraded piecemeal, so
// each step checks what is missing: tables, columns added after the first
// release, the messages table rebuilt for group chats, and read markers for
// chats older than unread tracking.
func upgradeUnversioned(tx *sql.Tx) error {
	hadReads := tableExists(tx, "conversation_reads")
	steps := []func() error{
		func() error { return exec(tx, schema) },
		func() error { return addColumns(tx) },
		func() error { return rebuildMessages(tx) },
		func() error { return exec(tx, lateIndexes) },
		func() error {
			if hadReads {
				return nil
			}
			return markOldChatsRead(tx)
		},
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return err
		}
	}
	return nil
}

func addColumns(tx *sql.Tx) error {
	for _, c := range addedColumns {
		if err := addColumn(tx, c.table, c.column, c.definition); err != nil {
			return err
		}
	}
	return nil
}

// addColumn adds a column unless the table already has it.
func addColumn(tx *sql.Tx, table, column, definition string) error {
	if columnExists(tx, table, column) {
		return nil
	}
	return exec(tx, `ALTER TABLE `+table+` ADD COLUMN `+column+` `+definition)
}

// rebuildMessages rebuilds a messages table from before group chats, where
// recipient_id was NOT NULL and there was no group_id (SQLite cannot relax a
// constraint in place). Nothing references messages, so the copy is safe.
func rebuildMessages(tx *sql.Tx) error {
	var notNull int
	if err := tx.QueryRow(`SELECT "notnull" FROM pragma_table_info('messages') WHERE name = 'recipient_id'`).Scan(&notNull); err != nil {
		return err
	}
	if notNull == 0 {
		return nil
	}
	return exec(tx, `
		CREATE TABLE messages_v2 (
			id           INTEGER PRIMARY KEY,
			sender_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			recipient_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
			group_id     INTEGER REFERENCES groups(id) ON DELETE CASCADE,
			body         TEXT NOT NULL,
			attachments  TEXT NOT NULL DEFAULT '[]',
			edited_at    TEXT,
			created_at   TEXT NOT NULL
		);
		INSERT INTO messages_v2(id, sender_id, recipient_id, group_id, body, attachments, edited_at, created_at)
			SELECT id, sender_id, recipient_id, NULL, body, attachments, edited_at, created_at FROM messages;
		DROP TABLE messages;
		ALTER TABLE messages_v2 RENAME TO messages;
		CREATE INDEX IF NOT EXISTS messages_sender ON messages(sender_id, recipient_id, id);
		CREATE INDEX IF NOT EXISTS messages_recipient ON messages(recipient_id, sender_id, id)`)
}

// markOldChatsRead: chats from before unread tracking start out read.
func markOldChatsRead(tx *sql.Tx) error {
	return exec(tx, `INSERT OR IGNORE INTO conversation_reads(user_id, peer_id, last_read_id)
		SELECT recipient_id, sender_id, MAX(id) FROM messages WHERE group_id IS NULL GROUP BY recipient_id, sender_id`)
}
