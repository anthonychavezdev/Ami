// Command server is the go-social backend: a single binary, SQLite on disk,
// JSON over HTTP.
package main

import (
	"database/sql"
	"flag"
	"log"
	"net/http"
	"time"
)

type server struct {
	db *sql.DB
}

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	dbPath := flag.String("db", "social.db", "sqlite database path")
	flag.Parse()

	db, err := openDB(*dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	s := &server{db: db}
	mux := http.NewServeMux()
	s.routes(mux)

	srv := &http.Server{
		Addr:              *addr,
		Handler:           cors(logRequests(mux)),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("go-social listening on %s (db %s)", *addr, *dbPath)
	log.Fatal(srv.ListenAndServe())
}

func (s *server) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// auth
	mux.HandleFunc("POST /api/register", s.register)
	mux.HandleFunc("POST /api/login", s.login)
	mux.HandleFunc("POST /api/logout", s.auth(s.logout))
	mux.HandleFunc("GET /api/me", s.auth(s.me))

	// users
	mux.HandleFunc("GET /api/users", s.auth(s.searchUsers))
	mux.HandleFunc("GET /api/users/{id}", s.auth(s.getUser))
	mux.HandleFunc("GET /api/users/{id}/posts", s.auth(s.userPosts))

	// posts, comments
	mux.HandleFunc("GET /api/feed", s.auth(s.feed))
	mux.HandleFunc("POST /api/posts", s.auth(s.createPost))
	mux.HandleFunc("GET /api/posts/{id}", s.auth(s.getPostHandler))
	mux.HandleFunc("DELETE /api/posts/{id}", s.auth(s.deletePost))
	mux.HandleFunc("GET /api/posts/{id}/comments", s.auth(s.listComments))
	mux.HandleFunc("POST /api/posts/{id}/comments", s.auth(s.createComment))
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, sw.status, time.Since(start).Round(time.Millisecond))
	})
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Access-Control-Allow-Origin", "*")
		h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
