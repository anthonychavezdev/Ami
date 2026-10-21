package main

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

// schema is what a new database gets: every table and index of the current
// version. An existing database is brought here by the migrations below.
// Ids are version 7 UUIDs as text (see newID in util.go): unguessable, and
// ordered by creation time, which every list and read marker relies on.
//
// Tables compare and sort bytes (utf8mb4_bin), the way SQLite did, so ids
// and tokens are exact; columns people search are case-insensitive
// (utf8mb4_unicode_ci), and so is the username's uniqueness. Timestamps are
// RFC 3339 strings in UTC, which sort as time.
const schema = `
CREATE TABLE IF NOT EXISTS users (
	id            CHAR(36) NOT NULL PRIMARY KEY,
	username      VARCHAR(24) NOT NULL COLLATE utf8mb4_unicode_ci,
	display_name  VARCHAR(50) NOT NULL COLLATE utf8mb4_unicode_ci,
	password_hash VARBINARY(64) NOT NULL,
	salt          VARBINARY(32) NOT NULL,
	password_iterations INT NOT NULL DEFAULT 210000, -- the count of the first release; login upgrades it (sessions.go)
	invisible     TINYINT(1) NOT NULL DEFAULT 0,
	avatar_url    VARCHAR(255) NOT NULL DEFAULT '',
	bio           TEXT NOT NULL DEFAULT '',
	pronouns      VARCHAR(30) NOT NULL DEFAULT '',
	avatar_crop   VARCHAR(255) NOT NULL DEFAULT '',
	must_change_password TINYINT(1) NOT NULL DEFAULT 0, -- set by -reset-password; cleared by PUT /api/me/password
	cover_url     VARCHAR(255) NOT NULL DEFAULT '',   -- the wide picture across the top of the profile
	cover_crop    VARCHAR(255) NOT NULL DEFAULT '',   -- its 3:1 window, JSON like avatar_crop
	interests     TEXT NOT NULL DEFAULT '[]',         -- JSON array of tags
	links         TEXT NOT NULL DEFAULT '[]',         -- JSON array of {label, url}
	created_at    VARCHAR(35) NOT NULL,
	UNIQUE KEY users_username (username)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS sessions (
	token      CHAR(64) NOT NULL PRIMARY KEY,
	user_id    CHAR(36) NOT NULL,
	expires_at VARCHAR(35) NOT NULL,
	KEY sessions_expires (expires_at), -- the hourly purge
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS groups (
	id          CHAR(36) NOT NULL PRIMARY KEY,
	name        VARCHAR(60) NOT NULL COLLATE utf8mb4_unicode_ci,
	description VARCHAR(500) NOT NULL DEFAULT '' COLLATE utf8mb4_unicode_ci,
	owner_id    CHAR(36) NOT NULL,
	private     TINYINT(1) NOT NULL DEFAULT 0, -- invite only: hidden from discovery, members are added by the admin
	created_at  VARCHAR(35) NOT NULL,
	FOREIGN KEY (owner_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS group_members (
	group_id  CHAR(36) NOT NULL,
	user_id   CHAR(36) NOT NULL,
	role      VARCHAR(10) NOT NULL CHECK (role IN ('admin', 'member')),
	joined_at VARCHAR(35) NOT NULL,
	PRIMARY KEY (group_id, user_id),
	KEY group_members_user (user_id),
	FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE CASCADE,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS posts (
	id          CHAR(36) NOT NULL PRIMARY KEY,
	author_id   CHAR(36) NOT NULL,
	group_id    CHAR(36) NULL,
	body        TEXT NOT NULL,
	attachments TEXT NOT NULL DEFAULT '[]',
	edited_at   VARCHAR(35) NULL,
	created_at  VARCHAR(35) NOT NULL,
	KEY posts_author (author_id, id),
	KEY posts_group (group_id, id),
	FOREIGN KEY (author_id) REFERENCES users(id) ON DELETE CASCADE,
	FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS comments (
	id         CHAR(36) NOT NULL PRIMARY KEY,
	post_id    CHAR(36) NOT NULL,
	author_id  CHAR(36) NOT NULL,
	parent_id  CHAR(36) NULL,
	body       TEXT NOT NULL,
	created_at VARCHAR(35) NOT NULL,
	edited_at  VARCHAR(35) NULL,
	deleted_at VARCHAR(35) NULL,
	KEY comments_post (post_id, id),
	KEY comments_parent (parent_id),
	FOREIGN KEY (post_id) REFERENCES posts(id) ON DELETE CASCADE,
	FOREIGN KEY (author_id) REFERENCES users(id) ON DELETE CASCADE,
	FOREIGN KEY (parent_id) REFERENCES comments(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS reactions (
	post_id CHAR(36) NOT NULL,
	user_id CHAR(36) NOT NULL,
	emoji   VARCHAR(32) NOT NULL,
	PRIMARY KEY (post_id, user_id),
	FOREIGN KEY (post_id) REFERENCES posts(id) ON DELETE CASCADE,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS comment_reactions (
	comment_id CHAR(36) NOT NULL,
	user_id    CHAR(36) NOT NULL,
	emoji      VARCHAR(32) NOT NULL,
	PRIMARY KEY (comment_id, user_id),
	FOREIGN KEY (comment_id) REFERENCES comments(id) ON DELETE CASCADE,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

-- One row per pair. from_id is whoever sent the request.
CREATE TABLE IF NOT EXISTS friendships (
	from_id    CHAR(36) NOT NULL,
	to_id      CHAR(36) NOT NULL,
	status     VARCHAR(10) NOT NULL CHECK (status IN ('pending', 'accepted')),
	created_at VARCHAR(35) NOT NULL,
	PRIMARY KEY (from_id, to_id),
	KEY friendships_to (to_id),
	FOREIGN KEY (from_id) REFERENCES users(id) ON DELETE CASCADE,
	FOREIGN KEY (to_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

-- A direct message has recipient_id, a group chat message has group_id.
CREATE TABLE IF NOT EXISTS messages (
	id           CHAR(36) NOT NULL PRIMARY KEY,
	sender_id    CHAR(36) NOT NULL,
	recipient_id CHAR(36) NULL,
	group_id     CHAR(36) NULL,
	body         TEXT NOT NULL,
	attachments  TEXT NOT NULL DEFAULT '[]',
	edited_at    VARCHAR(35) NULL,
	created_at   VARCHAR(35) NOT NULL,
	KEY messages_sender (sender_id, recipient_id, id),
	KEY messages_recipient (recipient_id, sender_id, id),
	KEY messages_group (group_id, id),
	FOREIGN KEY (sender_id) REFERENCES users(id) ON DELETE CASCADE,
	FOREIGN KEY (recipient_id) REFERENCES users(id) ON DELETE CASCADE,
	FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS message_reactions (
	message_id CHAR(36) NOT NULL,
	user_id    CHAR(36) NOT NULL,
	emoji      VARCHAR(32) NOT NULL,
	PRIMARY KEY (message_id, user_id),
	FOREIGN KEY (message_id) REFERENCES messages(id) ON DELETE CASCADE,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS group_reads (
	user_id      CHAR(36) NOT NULL,
	group_id     CHAR(36) NOT NULL,
	last_read_id VARCHAR(36) NOT NULL, -- the id of the last message read ('' for none)
	PRIMARY KEY (user_id, group_id),
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
	FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS conversation_reads (
	user_id      CHAR(36) NOT NULL,
	peer_id      CHAR(36) NOT NULL,
	last_read_id VARCHAR(36) NOT NULL,
	PRIMARY KEY (user_id, peer_id),
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
	FOREIGN KEY (peer_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

-- Reactions to an upload itself, a picture or video in the fullscreen viewer,
-- keyed by its file name: uploads are files, not rows.
CREATE TABLE IF NOT EXISTS media_reactions (
	media   VARCHAR(64) NOT NULL,
	user_id CHAR(36) NOT NULL,
	emoji   VARCHAR(32) NOT NULL,
	PRIMARY KEY (media, user_id),
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS push_tokens (
	token      VARCHAR(255) NOT NULL PRIMARY KEY,
	user_id    CHAR(36) NOT NULL,
	updated_at VARCHAR(35) NOT NULL,
	KEY push_tokens_user (user_id),
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS uploads (
	name       VARCHAR(64) NOT NULL PRIMARY KEY,
	user_id    CHAR(36) NOT NULL,
	size       BIGINT NOT NULL,
	created_at VARCHAR(35) NOT NULL,
	KEY uploads_user (user_id),
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS invites (
	code_hash  CHAR(64) NOT NULL PRIMARY KEY,
	created_at VARCHAR(35) NOT NULL,
	used_at    VARCHAR(35),
	used_by    CHAR(36),
	FOREIGN KEY (used_by) REFERENCES users(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
`

// A database records its schema version in the schema_version table. A new
// database gets `schema` and the latest version at once; an existing one
// runs every migration past its version, in order.
//
// To change the schema: edit `schema` (what new databases get) and append a
// migration that brings an existing database to the same place. The version
// number is the migration's position, so nothing else needs bumping. MariaDB
// commits DDL as it goes, so a migration that fails halfway leaves its
// earlier statements in place; write them so running again is harmless
// (addColumn and CREATE ... IF NOT EXISTS are).
var migrations = []migration{
	{"index for the session purge", func(tx *sql.Tx) error {
		return exec(tx, `CREATE INDEX IF NOT EXISTS sessions_expires ON sessions(expires_at)`)
	}},
	{"reactions on comments", func(tx *sql.Tx) error {
		return execEach(tx, schema) // every statement is CREATE ... IF NOT EXISTS: only comment_reactions is new
	}},
	{"reactions on messages", func(tx *sql.Tx) error {
		return execEach(tx, schema) // likewise: only message_reactions is new
	}},
	{"reactions on uploads", func(tx *sql.Tx) error {
		return execEach(tx, schema) // likewise: only media_reactions is new
	}},
	{"uploads are recorded with their owner and size", func(tx *sql.Tx) error {
		return execEach(tx, schema) // likewise: only uploads is new; the server fills it for older files at start (recordOldUploads)
	}},
	{"one-time invite codes", func(tx *sql.Tx) error {
		return execEach(tx, schema) // likewise: only invites is new
	}},
}

type migration struct {
	name string
	run  func(tx *sql.Tx) error
}

// openDB connects to MariaDB (user:password@tcp(host:3306)/database, or
// @unix(/path/mysql.sock)/database), waits up to half a minute for it to
// answer, since after a reboot it may still be starting, and brings the
// schema up to date.
func openDB(dsn string) (*sql.DB, error) {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("-db: %w", err)
	}
	cfg.Collation = "utf8mb4_unicode_ci" // utf8mb4 on the wire: posts and names carry emoji
	cfg.ParseTime = false                // timestamps are strings here
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(32)
	db.SetMaxIdleConns(8)
	db.SetConnMaxLifetime(5 * time.Minute)
	if err := waitFor(db, 30*time.Second); err != nil {
		db.Close()
		return nil, fmt.Errorf("database %s: %w", redactDSN(dsn), err)
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// waitFor pings the database until it answers or the time is up.
func waitFor(db *sql.DB, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		err := db.Ping()
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return err
		}
		log.Printf("db: waiting for the database (%v)", err)
		time.Sleep(time.Second)
	}
}

// redactDSN is a DSN without its password, for logs.
func redactDSN(dsn string) string {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return "?"
	}
	if cfg.Passwd != "" {
		cfg.Passwd = "..."
	}
	return cfg.FormatDSN()
}

// migrate brings a database of any earlier version up to date.
func migrate(db *sql.DB) error {
	if err := exec(db, `CREATE TABLE IF NOT EXISTS schema_version (version INT NOT NULL)`); err != nil {
		return err
	}
	version, err := schemaVersion(db)
	if err != nil {
		return err
	}
	if version == 0 && !tableExists(db, "users") { // a new database
		if err := execEach(db, schema); err != nil {
			return err
		}
		return setSchemaVersion(db, len(migrations))
	}
	for ; version < len(migrations); version++ {
		m := migrations[version]
		err := inTx(db, func(tx *sql.Tx) error {
			if err := m.run(tx); err != nil {
				return err
			}
			return setSchemaVersion(tx, version+1)
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

func exec(db dbtx, statement string, args ...any) error {
	_, err := db.Exec(statement, args...)
	return err
}

// execEach runs a script one statement at a time (the driver sends one per
// call), with the -- comments taken out.
func execEach(db dbtx, script string) error {
	var clean []string
	for _, line := range strings.Split(script, "\n") {
		if i := strings.Index(line, "--"); i >= 0 {
			line = line[:i]
		}
		clean = append(clean, line)
	}
	for _, statement := range strings.Split(strings.Join(clean, "\n"), ";") {
		if statement = strings.TrimSpace(statement); statement != "" {
			if err := exec(db, statement); err != nil {
				return fmt.Errorf("%w in: %.60s", err, statement)
			}
		}
	}
	return nil
}

func schemaVersion(db dbtx) (int, error) {
	var v int
	err := db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_version`).Scan(&v)
	return v, err
}

func setSchemaVersion(db dbtx, v int) error {
	if err := exec(db, `DELETE FROM schema_version`); err != nil {
		return err
	}
	return exec(db, `INSERT INTO schema_version(version) VALUES (?)`, v)
}

func tableExists(db dbtx, name string) bool {
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?`, name).Scan(&n)
	return n > 0
}

func columnExists(db dbtx, table, column string) bool {
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?`, table, column).Scan(&n)
	return n > 0
}

// addColumn adds a column unless the table already has it: the shape of
// most migrations.
func addColumn(db dbtx, table, column, definition string) error {
	if columnExists(db, table, column) {
		return nil
	}
	return exec(db, `ALTER TABLE `+table+` ADD COLUMN `+column+` `+definition)
}

// databaseSize is the bytes the database's tables and indexes take on disk.
func databaseSize(db dbtx) int64 {
	var n int64
	_ = db.QueryRow(`SELECT COALESCE(SUM(data_length + index_length), 0) FROM information_schema.tables WHERE table_schema = DATABASE()`).Scan(&n)
	return n
}

// isUniqueViolation is a lost race for a unique key, such as a username.
func isUniqueViolation(err error) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && me.Number == 1062
}
