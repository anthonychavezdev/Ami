// Command server is the Ami backend: a single binary, SQLite on disk,
// JSON over HTTP, and a WebSocket endpoint (gorilla/websocket) for realtime events
// and WebRTC signaling.
//
// Production is the default: no CORS, rate-limited auth endpoints, session
// tokens hashed at rest, strict timeouts. Development extras are opt-in (-dev).
package main

import (
	"bufio"
	"crypto/tls"
	"database/sql"
	"flag"
	"github.com/gorilla/websocket"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type server struct {
	db            *sql.DB
	hub           *hub
	limiter       *limiter // login/register attempts per IP
	apiLimiter    *limiter // all /api requests per IP (nil = off)
	uploadLimiter *limiter // uploads per user
	origins       map[string]bool
	upgrader      websocket.Upgrader
	mediaDir      string
	trustProxy    bool
}

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	dbPath := flag.String("db", "ami.db", "sqlite database path")
	mediaDir := flag.String("media-dir", "media", "directory for uploaded pictures and videos")
	dev := flag.Bool("dev", false, "development mode: seed demo data into an empty database and allow any CORS origin")
	origins := flag.String("allowed-origins", "", "comma-separated browser origins allowed to call the API (CORS); empty disables CORS")
	trustProxy := flag.Bool("trust-proxy", false, "take client IPs from X-Forwarded-For (only behind a trusted reverse proxy)")
	tlsCert := flag.String("tls-cert", "", "serve HTTPS with this certificate file (needs -tls-key)")
	tlsKey := flag.String("tls-key", "", "private key for -tls-cert")
	webDir := flag.String("web-dir", "", "serve the browser build from this directory (see `build web`)")
	apiRate := flag.Int("api-rate", 600, "requests per minute allowed per client IP on /api (0 disables)")
	flag.Parse()

	db, err := openDB(*dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if err := os.MkdirAll(*mediaDir, 0o755); err != nil {
		log.Fatalf("media dir: %v", err)
	}
	s := &server{db: db, hub: newHub(), limiter: newLimiter(10, time.Minute), uploadLimiter: newLimiter(30, time.Minute), mediaDir: *mediaDir, trustProxy: *trustProxy}
	if *dev {
		if err := s.seedIfEmpty(); err != nil {
			log.Fatalf("seed demo data: %v", err)
		}
	}
	s.origins = map[string]bool{}
	for _, o := range strings.Split(*origins, ",") {
		if o = strings.TrimSpace(o); o != "" {
			s.origins[o] = true
		}
	}
	if *dev && len(s.origins) == 0 {
		s.origins["*"] = true // e.g. the web app served by Metro on another port
	}
	if *dev {
		log.Printf("dev mode: demo data seeded, browser origins allowed: %s", *origins+map[bool]string{true: "*", false: ""}[s.origins["*"]])
	}
	if *apiRate > 0 {
		s.apiLimiter = newLimiter(*apiRate, time.Minute)
	}
	s.upgrader = websocket.Upgrader{ReadBufferSize: 4096, WriteBufferSize: 4096, CheckOrigin: s.originAllowed}
	go s.purgeSessions()
	mux := http.NewServeMux()
	s.routes(mux)
	if *webDir != "" {
		if _, err := os.Stat(filepath.Join(*webDir, "index.html")); err != nil {
			log.Fatalf("web dir: %v (build it with `build web`)", err)
		}
		mux.Handle("/", webApp(*webDir))
	}

	srv := &http.Server{
		Addr:              *addr,
		Handler:           s.secureHeaders(s.cors(logRequests(s.apiLimit(mux)))),
		MaxHeaderBytes:    64 << 10,
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
	mux.HandleFunc("PUT /api/posts/{id}", s.auth(s.updatePost))
	mux.HandleFunc("DELETE /api/posts/{id}", s.auth(s.deletePost))
	mux.HandleFunc("GET /api/posts/{id}/comments", s.auth(s.listComments))
	mux.HandleFunc("POST /api/posts/{id}/comments", s.auth(s.createComment))
	mux.HandleFunc("PUT /api/comments/{id}", s.auth(s.updateComment))
	mux.HandleFunc("DELETE /api/comments/{id}", s.auth(s.deleteComment))
	mux.HandleFunc("PUT /api/posts/{id}/reaction", s.auth(s.setReaction))
	mux.HandleFunc("DELETE /api/posts/{id}/reaction", s.auth(s.deleteReaction))
	mux.HandleFunc("POST /api/media", s.auth(s.uploadMedia))
	mux.HandleFunc("GET /media/{name}", s.serveMedia)

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
	mux.HandleFunc("POST /api/messages/{id}/read", s.auth(s.markRead))
	mux.HandleFunc("PUT /api/messages/{id}/{mid}", s.auth(s.updateMessage))
	mux.HandleFunc("PUT /api/push-token", s.auth(s.setPushToken))
	mux.HandleFunc("DELETE /api/push-token", s.auth(s.deletePushToken))

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

// Hijack lets gorilla/websocket take over the connection for /ws.
func (w *statusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return http.NewResponseController(w.ResponseWriter).Hijack()
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, sw.status, time.Since(start).Round(time.Millisecond))
	})
}

// originAllowed is the browser origin policy shared by CORS and the WebSocket
// upgrade: requests without an Origin (the native app), the page's own origin
// (the web build served by -web-dir) and the -allowed-origins list ("*" = any).
func (s *server) originAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" || s.origins["*"] || s.origins[origin] {
		return true
	}
	u, err := url.Parse(origin)
	return err == nil && strings.EqualFold(u.Host, r.Host)
}

// cors answers cross-origin browser requests for allowed origins. Other
// origins get no CORS headers (the browser then blocks the response) and a
// 403 on preflight.
func (s *server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Add("Vary", "Origin")
		if origin := r.Header.Get("Origin"); origin != "" {
			if !s.originAllowed(r) {
				if r.Method == http.MethodOptions {
					writeError(w, http.StatusForbidden, "origin not allowed")
					return
				}
			} else {
				h.Set("Access-Control-Allow-Origin", origin)
				h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
				h.Set("Access-Control-Max-Age", "600")
				if r.Method == http.MethodOptions {
					w.WriteHeader(http.StatusNoContent)
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

// apiLimit caps requests per client IP on the API as a whole; the login and
// register handlers have their own, much lower, budget.
func (s *server) apiLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.apiLimiter != nil && strings.HasPrefix(r.URL.Path, "/api/") && r.Method != http.MethodOptions && !s.apiLimiter.allow(s.clientIP(r)) {
			w.Header().Set("Retry-After", "60")
			writeError(w, http.StatusTooManyRequests, "too many requests")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// secureHeaders: API and media responses are never documents, so they get a
// locked-down CSP; the web build (web.go) and media (media.go) relax it.
func (s *server) secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Cache-Control", "no-store")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		h.Set("Permissions-Policy", "camera=(), microphone=(), display-capture=(), geolocation=()")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		if r.TLS != nil || (s.trustProxy && r.Header.Get("X-Forwarded-Proto") == "https") {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

// purgeSessions drops expired sessions once an hour (and once at start).
func (s *server) purgeSessions() {
	for {
		if _, err := s.db.Exec(`DELETE FROM sessions WHERE expires_at < ?`, nowString()); err != nil {
			log.Printf("purge expired sessions: %v", err)
		}
		time.Sleep(time.Hour)
	}
}
