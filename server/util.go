package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"
)

var errUnauthorized = errors.New("unauthorized")

func now() time.Time    { return time.Now().UTC() }
func nowString() string { return now().Format(time.RFC3339Nano) }

// presenceHook marks users as online/offline in every response (see presence.go).
var presenceHook func(v any) any

func writeJSON(w http.ResponseWriter, code int, v any) {
	if presenceHook != nil {
		v = presenceHook(v)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError sends {"error": msg}. Server-side failures are logged and
// replaced by a generic message so internals never reach clients.
func writeError(w http.ResponseWriter, code int, msg string) {
	if code >= 500 {
		log.Printf("error %d: %s", code, msg)
		msg = "internal error"
	}
	writeJSON(w, code, map[string]string{"error": msg})
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	return true
}

// pathID parses an integer path value; writes a 400 and returns false on failure.
func pathID(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid "+name)
		return 0, false
	}
	return id, true
}

func queryInt(r *http.Request, name string, def int64) int64 {
	if v, err := strconv.ParseInt(r.URL.Query().Get(name), 10, 64); err == nil && v > 0 {
		return v
	}
	return def
}

// pagination reads ?before=<id>&limit=<n> cursor parameters.
func pagination(r *http.Request) (before, limit int64) {
	before = queryInt(r, "before", 1<<62)
	limit = queryInt(r, "limit", 30)
	if limit > 100 {
		limit = 100
	}
	return
}

func clampLen(s string, max int) bool { return len(s) > 0 && len(s) <= max }
