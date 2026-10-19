package main

import (
	"database/sql"
	"fmt"
	"log"

	_ "modernc.org/sqlite" // pure-Go SQLite driver (the only non-stdlib dependency)
)

// schema is what a new database gets: every table and index of the current
// version. An existing database is brought here by the migrations below.
// Ids are version 7 UUIDs as text (see newID in util.go): unguessable, and
// ordered by creation time, which every list and read marker relies on.
const schema = `
CREATE TABLE IF NOT EXISTS users (
	id            TEXT PRIMARY KEY,
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
	cover_url     TEXT NOT NULL DEFAULT '',   -- the wide picture across the top of the profile
	cover_crop    TEXT NOT NULL DEFAULT '',   -- its 3:1 window, JSON like avatar_crop
	interests     TEXT NOT NULL DEFAULT '[]', -- JSON array of tags
	links         TEXT NOT NULL DEFAULT '[]', -- JSON array of {label, url}
	created_at    TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
	token      TEXT PRIMARY KEY,
	user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	expires_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS groups (
	id          TEXT PRIMARY KEY,
	name        TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	owner_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	private     INTEGER NOT NULL DEFAULT 0, -- invite only: hidden from discovery, members are added by the admin
	created_at  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS group_members (
	group_id  TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
	user_id   TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	role      TEXT NOT NULL CHECK (role IN ('admin', 'member')),
	joined_at TEXT NOT NULL,
	PRIMARY KEY (group_id, user_id)
);
CREATE INDEX IF NOT EXISTS group_members_user ON group_members(user_id);

CREATE TABLE IF NOT EXISTS posts (
	id         TEXT PRIMARY KEY,
	author_id  TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	group_id   TEXT REFERENCES groups(id) ON DELETE CASCADE,
	body       TEXT NOT NULL,
	attachments TEXT NOT NULL DEFAULT '[]',
	edited_at   TEXT,
	created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS posts_author ON posts(author_id, id);
CREATE INDEX IF NOT EXISTS posts_group ON posts(group_id, id);

CREATE TABLE IF NOT EXISTS comments (
	id         TEXT PRIMARY KEY,
	post_id    TEXT NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
	author_id  TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	parent_id  TEXT REFERENCES comments(id),
	body       TEXT NOT NULL,
	created_at TEXT NOT NULL,
	edited_at  TEXT,
	deleted_at TEXT
);
CREATE INDEX IF NOT EXISTS comments_post ON comments(post_id, id);

CREATE TABLE IF NOT EXISTS reactions (
	post_id TEXT NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
	user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	emoji   TEXT NOT NULL,
	PRIMARY KEY (post_id, user_id)
);

-- One row per pair. from_id is whoever sent the request.
CREATE TABLE IF NOT EXISTS friendships (
	from_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	to_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	status     TEXT NOT NULL CHECK (status IN ('pending', 'accepted')),
	created_at TEXT NOT NULL,
	PRIMARY KEY (from_id, to_id)
);
CREATE INDEX IF NOT EXISTS friendships_to ON friendships(to_id);

-- A direct message has recipient_id, a group chat message has group_id.
CREATE TABLE IF NOT EXISTS messages (
	id           TEXT PRIMARY KEY,
	sender_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	recipient_id TEXT REFERENCES users(id) ON DELETE CASCADE,
	group_id     TEXT REFERENCES groups(id) ON DELETE CASCADE,
	body         TEXT NOT NULL,
	attachments  TEXT NOT NULL DEFAULT '[]',
	edited_at    TEXT,
	created_at   TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS messages_sender ON messages(sender_id, recipient_id, id);
CREATE INDEX IF NOT EXISTS messages_recipient ON messages(recipient_id, sender_id, id);

CREATE TABLE IF NOT EXISTS group_reads (
	user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	group_id     TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
	last_read_id TEXT NOT NULL, -- the id of the last message read ('' for none)
	PRIMARY KEY (user_id, group_id)
);

CREATE TABLE IF NOT EXISTS conversation_reads (
	user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	peer_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	last_read_id TEXT NOT NULL,
	PRIMARY KEY (user_id, peer_id)
);

CREATE TABLE IF NOT EXISTS push_tokens (
	token      TEXT PRIMARY KEY,
	user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
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
	{"ids become UUIDs", toUUIDs},
	{"cover, interests and links", func(tx *sql.Tx) error {
		for _, c := range []struct{ column, definition string }{
			{"cover_url", "TEXT NOT NULL DEFAULT ''"}, {"cover_crop", "TEXT NOT NULL DEFAULT ''"},
			{"interests", "TEXT NOT NULL DEFAULT '[]'"}, {"links", "TEXT NOT NULL DEFAULT '[]'"},
		} {
			if err := addColumn(tx, "users", c.column, c.definition); err != nil {
				return err
			}
		}
		return nil
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

// columnType is a column's declared type ("TEXT", "INTEGER"), or "" if absent.
func columnType(db dbtx, table, column string) string {
	var t string
	_ = db.QueryRow(`SELECT type FROM pragma_table_info(?) WHERE name = ?`, table, column).Scan(&t)
	return t
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

// ---- migration 3

// tables is every table, parents first; idTables are the ones with an id of
// their own that other tables point at.
var (
	tables   = []string{"users", "sessions", "groups", "group_members", "posts", "comments", "reactions", "friendships", "messages", "group_reads", "conversation_reads", "push_tokens"}
	idTables = []string{"users", "groups", "posts", "comments", "messages"}
)

// toUUIDs turns the integer ids of every table into UUIDs, keeping every
// reference: the tables are copied aside, dropped, recreated from the schema
// and refilled through a map from each old id to a new one. New ids are
// handed out in the old order, so what was newer stays newer, and a read
// marker lands on the newest message at or before where it was. A database
// whose ids are already text is left alone.
func toUUIDs(tx *sql.Tx) error {
	if columnType(tx, "users", "id") == "TEXT" {
		return nil
	}
	if err := exec(tx, `PRAGMA defer_foreign_keys = ON`); err != nil {
		return err
	}
	for _, t := range tables {
		if err := exec(tx, `CREATE TEMP TABLE old_`+t+` AS SELECT * FROM `+t); err != nil {
			return err
		}
	}
	for _, t := range idTables {
		if err := mapIDs(tx, t); err != nil {
			return err
		}
	}
	for i := len(tables) - 1; i >= 0; i-- { // children first, so nothing cascades
		if err := exec(tx, `DROP TABLE `+tables[i]); err != nil {
			return err
		}
	}
	if err := exec(tx, schema+lateIndexes+refill); err != nil {
		return err
	}
	for _, t := range tables {
		if err := exec(tx, `DROP TABLE old_`+t); err != nil {
			return err
		}
	}
	for _, t := range idTables {
		if err := exec(tx, `DROP TABLE map_`+t); err != nil {
			return err
		}
	}
	return nil
}

// mapIDs fills map_<table>(old, new) with a fresh id for every row, in id order.
func mapIDs(tx *sql.Tx, table string) error {
	if err := exec(tx, `CREATE TEMP TABLE map_`+table+`(old INTEGER PRIMARY KEY, new TEXT NOT NULL)`); err != nil {
		return err
	}
	rows, err := tx.Query(`SELECT CAST(id AS INTEGER) FROM old_` + table + ` ORDER BY CAST(id AS INTEGER)`)
	if err != nil {
		return err
	}
	var olds []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		olds = append(olds, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, old := range olds {
		if _, err := tx.Exec(`INSERT INTO map_`+table+`(old, new) VALUES (?, ?)`, old, newID()); err != nil {
			return err
		}
	}
	return nil
}

// refill copies every old table into its new shape through the id maps. Old
// id columns are cast, since tables created by migration 1 already had TEXT
// columns holding integers. A read marker becomes the new id of the newest
// message at or before it, or ” when there is none.
const refill = `
INSERT INTO users(id, username, display_name, password_hash, salt, password_iterations, invisible, avatar_url, bio, pronouns, avatar_crop, must_change_password, created_at)
	SELECT m.new, o.username, o.display_name, o.password_hash, o.salt, o.password_iterations, o.invisible, o.avatar_url, o.bio, o.pronouns, o.avatar_crop, o.must_change_password, o.created_at
	FROM old_users o JOIN map_users m ON m.old = CAST(o.id AS INTEGER);
INSERT INTO sessions(token, user_id, expires_at)
	SELECT o.token, mu.new, o.expires_at FROM old_sessions o JOIN map_users mu ON mu.old = CAST(o.user_id AS INTEGER);
INSERT INTO groups(id, name, description, owner_id, private, created_at)
	SELECT m.new, o.name, o.description, mu.new, o.private, o.created_at
	FROM old_groups o JOIN map_groups m ON m.old = CAST(o.id AS INTEGER) JOIN map_users mu ON mu.old = CAST(o.owner_id AS INTEGER);
INSERT INTO group_members(group_id, user_id, role, joined_at)
	SELECT mg.new, mu.new, o.role, o.joined_at
	FROM old_group_members o JOIN map_groups mg ON mg.old = CAST(o.group_id AS INTEGER) JOIN map_users mu ON mu.old = CAST(o.user_id AS INTEGER);
INSERT INTO posts(id, author_id, group_id, body, attachments, edited_at, created_at)
	SELECT m.new, mu.new, mg.new, o.body, o.attachments, o.edited_at, o.created_at
	FROM old_posts o JOIN map_posts m ON m.old = CAST(o.id AS INTEGER) JOIN map_users mu ON mu.old = CAST(o.author_id AS INTEGER)
	LEFT JOIN map_groups mg ON mg.old = CAST(o.group_id AS INTEGER);
INSERT INTO comments(id, post_id, author_id, parent_id, body, created_at, edited_at, deleted_at)
	SELECT m.new, mp.new, mu.new, mc.new, o.body, o.created_at, o.edited_at, o.deleted_at
	FROM old_comments o JOIN map_comments m ON m.old = CAST(o.id AS INTEGER) JOIN map_posts mp ON mp.old = CAST(o.post_id AS INTEGER)
	JOIN map_users mu ON mu.old = CAST(o.author_id AS INTEGER) LEFT JOIN map_comments mc ON mc.old = CAST(o.parent_id AS INTEGER);
INSERT INTO reactions(post_id, user_id, emoji)
	SELECT mp.new, mu.new, o.emoji
	FROM old_reactions o JOIN map_posts mp ON mp.old = CAST(o.post_id AS INTEGER) JOIN map_users mu ON mu.old = CAST(o.user_id AS INTEGER);
INSERT INTO friendships(from_id, to_id, status, created_at)
	SELECT mf.new, mt.new, o.status, o.created_at
	FROM old_friendships o JOIN map_users mf ON mf.old = CAST(o.from_id AS INTEGER) JOIN map_users mt ON mt.old = CAST(o.to_id AS INTEGER);
INSERT INTO messages(id, sender_id, recipient_id, group_id, body, attachments, edited_at, created_at)
	SELECT m.new, ms.new, mr.new, mg.new, o.body, o.attachments, o.edited_at, o.created_at
	FROM old_messages o JOIN map_messages m ON m.old = CAST(o.id AS INTEGER) JOIN map_users ms ON ms.old = CAST(o.sender_id AS INTEGER)
	LEFT JOIN map_users mr ON mr.old = CAST(o.recipient_id AS INTEGER) LEFT JOIN map_groups mg ON mg.old = CAST(o.group_id AS INTEGER);
INSERT INTO group_reads(user_id, group_id, last_read_id)
	SELECT mu.new, mg.new, COALESCE((SELECT mm.new FROM map_messages mm WHERE mm.old <= CAST(o.last_read_id AS INTEGER) ORDER BY mm.old DESC LIMIT 1), '')
	FROM old_group_reads o JOIN map_users mu ON mu.old = CAST(o.user_id AS INTEGER) JOIN map_groups mg ON mg.old = CAST(o.group_id AS INTEGER);
INSERT INTO conversation_reads(user_id, peer_id, last_read_id)
	SELECT mu.new, mp.new, COALESCE((SELECT mm.new FROM map_messages mm WHERE mm.old <= CAST(o.last_read_id AS INTEGER) ORDER BY mm.old DESC LIMIT 1), '')
	FROM old_conversation_reads o JOIN map_users mu ON mu.old = CAST(o.user_id AS INTEGER) JOIN map_users mp ON mp.old = CAST(o.peer_id AS INTEGER);
INSERT INTO push_tokens(token, user_id, updated_at)
	SELECT o.token, mu.new, o.updated_at FROM old_push_tokens o JOIN map_users mu ON mu.old = CAST(o.user_id AS INTEGER);
`
