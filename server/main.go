// Command server is the Ami backend: a single binary, SQLite on disk,
// JSON over HTTP, and a hand-rolled WebSocket endpoint for realtime events
// and WebRTC signaling.
//
// Production is the default: no CORS, rate-limited auth endpoints, session
// tokens hashed at rest, strict timeouts. Development extras are opt-in (-dev).
package main

import (
	"crypto/tls"
	"database/sql"
	"flag"
	"log"
	"net/http"
	"strings"
	"time"
)

type server struct {
	db         *sql.DB
	hub        *hub
	limiter    *limiter
	trustProxy bool
}

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	dbPath := flag.String("db", "ami.db", "sqlite database path")
	dev := flag.Bool("dev", false, "development mode: seed demo data into an empty database and allow any CORS origin")
	origins := flag.String("allowed-origins", "", "comma-separated browser origins allowed to call the API (CORS); empty disables CORS")
	trustProxy := flag.Bool("trust-proxy", false, "take client IPs from X-Forwarded-For (only behind a trusted reverse proxy)")
	tlsCert := flag.String("tls-cert", "", "serve HTTPS with this certificate file (needs -tls-key)")
	tlsKey := flag.String("tls-key", "", "private key for -tls-cert")
	flag.Parse()

	db, err := openDB(*dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	s := &server{db: db, hub: newHub(), limiter: newLimiter(10, time.Minute), trustProxy: *trustProxy}
	if _, err := db.Exec(`DELETE FROM sessions WHERE expires_at < ?`, nowString()); err != nil {
		log.Printf("purge expired sessions: %v", err)
	}
	if *dev {
		if err := s.seedIfEmpty(); err != nil {
			log.Fatalf("seed demo data: %v", err)
		}
	}
	allowed := strings.Split(*origins, ",")
	if *dev {
		allowed = []string{"*"}
	}
	mux := http.NewServeMux()
	s.routes(mux)

	srv := &http.Server{
		Addr:              *addr,
		Handler:           secureHeaders(cors(allowed, logRequests(mux))),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       2 * time.Minute,
		TLSConfig:         &tls.Config{MinVersion: tls.VersionTLS12},
	}
	log.Printf("ami listening on %s (db %s)", *addr, *dbPath)
	if *tlsCert != "" || *tlsKey != "" {
		log.Fatal(srv.ListenAndServeTLS(*tlsCert, *tlsKey))
	}
	log.Fatal(srv.ListenAndServe())
}

func (s *server) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// auth
	mux.HandleFunc("POST /api/register", s.throttle(s.register))
	mux.HandleFunc("POST /api/login", s.throttle(s.login))
	mux.HandleFunc("POST /api/logout", s.auth(s.logout))
	mux.HandleFunc("GET /api/me", s.auth(s.me))

	// users
	mux.HandleFunc("GET /api/users", s.auth(s.searchUsers))
	mux.HandleFunc("GET /api/users/{id}", s.auth(s.getUser))
	mux.HandleFunc("GET /api/users/{id}/posts", s.auth(s.userPosts))

	// posts, comments, reactions
	mux.HandleFunc("GET /api/feed", s.auth(s.feed))
	mux.HandleFunc("POST /api/posts", s.auth(s.createPost))
	mux.HandleFunc("GET /api/posts/{id}", s.auth(s.getPostHandler))
	mux.HandleFunc("DELETE /api/posts/{id}", s.auth(s.deletePost))
	mux.HandleFunc("GET /api/posts/{id}/comments", s.auth(s.listComments))
	mux.HandleFunc("POST /api/posts/{id}/comments", s.auth(s.createComment))
	mux.HandleFunc("PUT /api/posts/{id}/reaction", s.auth(s.setReaction))
	mux.HandleFunc("DELETE /api/posts/{id}/reaction", s.auth(s.deleteReaction))

	// friends
	mux.HandleFunc("GET /api/friends", s.auth(s.listFriends))
	mux.HandleFunc("GET /api/friends/requests", s.auth(s.listFriendRequests))
	mux.HandleFunc("POST /api/friends/{id}/request", s.auth(s.sendFriendRequest))
	mux.HandleFunc("POST /api/friends/{id}/accept", s.auth(s.acceptFriendRequest))
	mux.HandleFunc("DELETE /api/friends/{id}", s.auth(s.removeFriend))

	// groups
	mux.HandleFunc("GET /api/groups", s.auth(s.listGroups))
	mux.HandleFunc("POST /api/groups", s.auth(s.createGroup))
	mux.HandleFunc("GET /api/groups/{id}", s.auth(s.getGroupHandler))
	mux.HandleFunc("DELETE /api/groups/{id}", s.auth(s.deleteGroup))
	mux.HandleFunc("POST /api/groups/{id}/join", s.auth(s.joinGroup))
	mux.HandleFunc("POST /api/groups/{id}/leave", s.auth(s.leaveGroup))
	mux.HandleFunc("GET /api/groups/{id}/posts", s.auth(s.groupPosts))
	mux.HandleFunc("DELETE /api/groups/{id}/members/{uid}", s.auth(s.removeGroupMember))

	// direct messages
	mux.HandleFunc("GET /api/conversations", s.auth(s.listConversations))
	mux.HandleFunc("GET /api/messages/{id}", s.auth(s.listMessages))
	mux.HandleFunc("POST /api/messages/{id}", s.auth(s.sendMessage))

	// realtime: events + call signaling
	mux.HandleFunc("GET /ws", s.handleWS)
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// Unwrap lets http.ResponseController reach the underlying Hijacker.
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, sw.status, time.Since(start).Round(time.Millisecond))
	})
}

// cors answers browser cross-origin requests for the allowed origins ("*" for
// any). With none configured no CORS headers are sent, which is all the native
// app needs.
func cors(allowed []string, next http.Handler) http.Handler {
	set := map[string]bool{}
	for _, o := range allowed {
		if o = strings.TrimSpace(o); o != "" {
			set[o] = true
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" && (set["*"] || set[origin]) {
			h := w.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			h.Add("Vary", "Origin")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Cache-Control", "no-store")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}
