package main

import "testing"

// A new database gets the whole schema and the current version; a second
// start finds everything in place; the helpers migrations rely on see what
// exists and add a column only once.
func TestNewDatabaseIsCurrent(t *testing.T) {
	dsn := testDatabase(t)
	db, err := openDB(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if v, err := schemaVersion(db); err != nil || v != len(migrations) {
		t.Fatalf("version %d, %v; want %d", v, err, len(migrations))
	}
	for _, table := range []string{"users", "sessions", "groups", "group_members", "posts", "comments", "reactions", "friendships", "messages", "group_reads", "conversation_reads", "push_tokens"} {
		if !tableExists(db, table) {
			t.Errorf("table %s missing", table)
		}
	}
	if !columnExists(db, "users", "links") || columnExists(db, "users", "nope") {
		t.Error("columnExists is wrong")
	}
	again, err := openDB(dsn)
	if err != nil {
		t.Fatal("second start:", err)
	}
	again.Close()
	for i := 0; i < 2; i++ {
		if err := addColumn(db, "users", "extra", "INT NOT NULL DEFAULT 0"); err != nil {
			t.Fatalf("addColumn, time %d: %v", i+1, err)
		}
	}
	if !columnExists(db, "users", "extra") {
		t.Error("extra column missing")
	}
	if databaseSize(db) <= 0 {
		t.Error("databaseSize is 0")
	}
}
