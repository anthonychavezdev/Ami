package main

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// The schema of the first release, before columns were added, group chats
// changed the messages table and read markers appeared.
const firstReleaseSchema = `
CREATE TABLE users (
	id            INTEGER PRIMARY KEY,
	username      TEXT NOT NULL UNIQUE COLLATE NOCASE,
	display_name  TEXT NOT NULL,
	password_hash BLOB NOT NULL,
	salt          BLOB NOT NULL,
	created_at    TEXT NOT NULL
);
CREATE TABLE sessions (token TEXT PRIMARY KEY, user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE, expires_at TEXT NOT NULL);
CREATE TABLE groups (
	id INTEGER PRIMARY KEY, name TEXT NOT NULL, description TEXT NOT NULL DEFAULT '',
	owner_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE, created_at TEXT NOT NULL
);
CREATE TABLE group_members (
	group_id INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE, user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	role TEXT NOT NULL CHECK (role IN ('admin', 'member')), joined_at TEXT NOT NULL, PRIMARY KEY (group_id, user_id)
);
CREATE TABLE posts (
	id INTEGER PRIMARY KEY, author_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	group_id INTEGER REFERENCES groups(id) ON DELETE CASCADE, body TEXT NOT NULL, created_at TEXT NOT NULL
);
CREATE TABLE comments (
	id INTEGER PRIMARY KEY, post_id INTEGER NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
	author_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE, body TEXT NOT NULL, created_at TEXT NOT NULL
);
CREATE TABLE reactions (post_id INTEGER NOT NULL, user_id INTEGER NOT NULL, emoji TEXT NOT NULL, PRIMARY KEY (post_id, user_id));
CREATE TABLE friendships (
	from_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE, to_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	status TEXT NOT NULL CHECK (status IN ('pending', 'accepted')), created_at TEXT NOT NULL, PRIMARY KEY (from_id, to_id)
);
CREATE TABLE messages (
	id INTEGER PRIMARY KEY, sender_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	recipient_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE, body TEXT NOT NULL, created_at TEXT NOT NULL
);
INSERT INTO users(id, username, display_name, password_hash, salt, created_at) VALUES (1, 'ana', 'Ana', x'00', x'00', '2024-01-01T00:00:00Z'), (2, 'luis', 'Luis', x'00', x'00', '2024-01-01T00:00:00Z');
INSERT INTO friendships VALUES (1, 2, 'accepted', '2024-01-01T00:00:00Z');
INSERT INTO messages(sender_id, recipient_id, body, created_at) VALUES (1, 2, 'hola', '2024-01-02T00:00:00Z'), (2, 1, 'hola ana', '2024-01-02T00:01:00Z');
INSERT INTO posts(author_id, body, created_at) VALUES (1, 'first', '2024-01-02T00:00:00Z');
`

func TestMigrateFirstRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	old, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec(firstReleaseSchema); err != nil {
		t.Fatal(err)
	}
	old.Close()

	db, err := openDB(path)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	defer db.Close()

	v, _ := userVersion(db)
	if v != len(migrations) {
		t.Errorf("version after migrating: %d, want %d", v, len(migrations))
	}
	for _, c := range addedColumns {
		if !columnExists(db, c.table, c.column) {
			t.Errorf("column %s.%s missing", c.table, c.column)
		}
	}
	var notNull int
	_ = db.QueryRow(`SELECT "notnull" FROM pragma_table_info('messages') WHERE name = 'recipient_id'`).Scan(&notNull)
	if notNull != 0 || !columnExists(db, "messages", "group_id") {
		t.Errorf("messages table was not rebuilt for group chats (recipient_id notnull=%d)", notNull)
	}
	var kept int
	_ = db.QueryRow(`SELECT COUNT(*) FROM messages WHERE group_id IS NULL`).Scan(&kept)
	if kept != 2 {
		t.Errorf("messages lost in the rebuild: %d of 2 left", kept)
	}
	var reads int
	_ = db.QueryRow(`SELECT COUNT(*) FROM conversation_reads`).Scan(&reads)
	if reads != 2 {
		t.Errorf("old chats should start out read: %d read markers, want 2", reads)
	}
	for _, table := range []string{"conversation_reads", "group_reads", "push_tokens"} {
		if !tableExists(db, table) {
			t.Errorf("table %s missing", table)
		}
	}
	if !columnExists(db, "users", "must_change_password") {
		t.Error("migration 2 did not run after migration 1")
	}
	if _, err := db.Exec(`INSERT INTO messages(sender_id, group_id, body, created_at) VALUES (1, NULL, 'x', '2024-01-03T00:00:00Z')`); err != nil {
		t.Errorf("insert after migration: %v", err)
	}
	// Opening again is a no-op.
	if err := migrate(db); err != nil {
		t.Errorf("second migrate: %v", err)
	}
}

func TestNewDatabaseIsCurrent(t *testing.T) {
	db, err := openDB(filepath.Join(t.TempDir(), "new.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	v, _ := userVersion(db)
	if v != len(migrations) {
		t.Errorf("new database at version %d, want %d", v, len(migrations))
	}
	for _, c := range addedColumns {
		if !columnExists(db, c.table, c.column) {
			t.Errorf("schema lacks %s.%s, which addedColumns adds to old databases", c.table, c.column)
		}
	}
	if err := migrate(db); err != nil {
		t.Errorf("migrate on a current database: %v", err)
	}
}

// Later migrations are checked by taking a current database back one version.
func TestMigrateFromVersion1(t *testing.T) {
	db, err := openDB(filepath.Join(t.TempDir(), "v1.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`ALTER TABLE users DROP COLUMN must_change_password`); err != nil {
		t.Fatal(err)
	}
	if err := setUserVersion(db, 1); err != nil {
		t.Fatal(err)
	}
	if err := migrate(db); err != nil {
		t.Fatalf("migrate from version 1: %v", err)
	}
	v, _ := userVersion(db)
	if v != len(migrations) || !columnExists(db, "users", "must_change_password") {
		t.Errorf("version %d, must_change_password present: %v", v, columnExists(db, "users", "must_change_password"))
	}
}
