package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"os"
	"strings"
)

var errUnauthorized = errors.New("unauthorized")

func now() time.Time    { return time.Now().UTC() }
func nowString() string { return now().Format(time.RFC3339Nano) }

// ---- ids
//
// Every row has a version 7 UUID for its id, made here: random, so nothing
// can be guessed or counted, but ordered by creation time, so "newer" is
// still "greater" for every list, cursor and read marker. Ids are stored and
// compared as their canonical text (lowercase, hyphenated, 36 characters).

// maxID sorts after every id: the cursor for "the newest page".
const maxID = "ffffffff-ffff-ffff-ffff-ffffffffffff"

func newID() string {
	id, err := uuid.NewV7()
	if err != nil {
		panic(err) // only when the random source fails, which nothing recovers from
	}
	return id.String()
}

// isID says whether s is an id in the form the database stores.
func isID(s string) bool {
	u, err := uuid.Parse(s)
	return err == nil && u.String() == s
}

// presenceHook marks users as online/offline in every response (see presence.go).
var presenceHook func(v any) any

// ---- handlers and their errors
//
// A handler returns an error instead of writing one. An *httpError becomes
// that status and message; any other error is logged and answered with a
// generic 500, so internals never reach clients.

type httpError struct {
	code int
	msg  string
}

func (e *httpError) Error() string { return e.msg }

func fail(code int, msg string) error { return &httpError{code: code, msg: msg} }
func badRequest(msg string) error     { return fail(http.StatusBadRequest, msg) }
func forbidden(msg string) error      { return fail(http.StatusForbidden, msg) }
func notFound(msg string) error       { return fail(http.StatusNotFound, msg) }
func conflict(msg string) error       { return fail(http.StatusConflict, msg) }

// orNotFound turns "no rows" into a 404 with msg and leaves other errors alone.
func orNotFound(err error, msg string) error {
	if errors.Is(err, sql.ErrNoRows) {
		return notFound(msg)
	}
	return err
}

type handler func(w http.ResponseWriter, r *http.Request) error

// public adapts a handler that needs no session.
func public(h handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { respond(w, h(w, r)) }
}

// respond writes a handler's error, if there is one.
func respond(w http.ResponseWriter, err error) {
	var he *httpError
	switch {
	case err == nil:
	case errors.As(err, &he):
		writeJSON(w, he.code, map[string]string{"error": he.msg})
	default:
		log.Printf("error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
	}
}

// writeError answers from middleware, before any handler runs.
func writeError(w http.ResponseWriter, code int, msg string) { respond(w, fail(code, msg)) }

func writeJSON(w http.ResponseWriter, code int, v any) {
	if presenceHook != nil {
		v = presenceHook(v)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// ok, created and noContent finish a handler with a response and no error.
func ok(w http.ResponseWriter, v any) error      { writeJSON(w, http.StatusOK, v); return nil }
func created(w http.ResponseWriter, v any) error { writeJSON(w, http.StatusCreated, v); return nil }
func noContent(w http.ResponseWriter) error      { w.WriteHeader(http.StatusNoContent); return nil }

func readJSON(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		return badRequest("invalid JSON body")
	}
	return nil
}

// pathID reads a path value that must be an id.
func pathID(r *http.Request, name string) (string, error) {
	id := r.PathValue(name)
	if !isID(id) {
		return "", badRequest("invalid " + name)
	}
	return id, nil
}

// queryID reads an id parameter, or def when it is absent or malformed.
func queryID(r *http.Request, name, def string) string {
	if v := r.URL.Query().Get(name); isID(v) {
		return v
	}
	return def
}

func queryInt(r *http.Request, name string, def int64) int64 {
	if v, err := strconv.ParseInt(r.URL.Query().Get(name), 10, 64); err == nil && v > 0 {
		return v
	}
	return def
}

// pagination reads ?before=<id>&limit=<n> cursor parameters: items older
// than the one with id before (by default, everything), at most limit.
func pagination(r *http.Request) (before string, limit int64) {
	return queryID(r, "before", maxID), min(queryInt(r, "limit", 30), 100)
}

func clampLen(s string, max int) bool { return len(s) > 0 && len(s) <= max }

// unreadCount is the answer of the read-marker routes.
type unreadCount struct {
	Unread int `json:"unread"`
}

// jsonString encodes v for a JSON text column.
func jsonString(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// mapSlice applies f to every element.
func mapSlice[T, U any](xs []T, f func(T) U) []U {
	out := make([]U, 0, len(xs))
	for _, x := range xs {
		out = append(out, f(x))
	}
	return out
}

// without is xs minus every occurrence of x.
func without[T comparable](xs []T, x T) []T {
	out := make([]T, 0, len(xs))
	for _, v := range xs {
		if v != x {
			out = append(out, v)
		}
	}
	return out
}

// ---- database helpers

// fields joins groups of scan targets, so a row that spans several structs
// (a message and its sender, say) is scanned through each struct's own list.
func fields(groups ...[]any) []any {
	var out []any
	for _, g := range groups {
		out = append(out, g...)
	}
	return out
}

type scanner = interface{ Scan(dest ...any) error }

// queryAll collects every row of a query through scan.
func queryAll[T any](db *sql.DB, scan func(scanner) (T, error), query string, args ...any) ([]T, error) {
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []T{}
	for rows.Next() {
		v, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func scanID(sc scanner) (string, error) {
	var id string
	return id, sc.Scan(&id)
}

// queryIDs collects a single id column; a failing query reads as empty.
func queryIDs(db *sql.DB, query string, args ...any) []string {
	ids, _ := queryAll(db, scanID, query, args...)
	return ids
}

// inTx runs fn in a transaction that is committed when fn returns nil.
func (s *server) inTx(fn func(tx *sql.Tx) error) error { return inTx(s.db, fn) }

func inTx(db *sql.DB, fn func(tx *sql.Tx) error) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// envOr is an environment variable, or def when it is unset or empty.
func envOr(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

// placeholders is "?, ?, ?" for an IN list of n values.
func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?, ", n), ", ")
}
