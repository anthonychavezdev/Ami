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
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/websocket"
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
	calls         *callRooms // who is in which call
}

// config is everything the command-line flags decide.
type config struct {
	addr, dbPath, mediaDir, origins, tlsCert, tlsKey, webDir string
	dev, trustProxy                                          bool
	apiRate                                                  int
	resetPassword                                            string // a username: print a temporary password for it and exit
}

func parseFlags() config {
	var c config
	flag.StringVar(&c.addr, "addr", ":8080", "listen address")
	flag.StringVar(&c.dbPath, "db", "ami.db", "sqlite database path")
	flag.StringVar(&c.mediaDir, "media-dir", "media", "directory for uploaded pictures and videos")
	flag.BoolVar(&c.dev, "dev", false, "development mode: seed demo data into an empty database and allow any CORS origin")
	flag.StringVar(&c.origins, "allowed-origins", "", "comma-separated browser origins allowed to call the API (CORS); empty disables CORS")
	flag.BoolVar(&c.trustProxy, "trust-proxy", false, "take client IPs from X-Forwarded-For (only behind a trusted reverse proxy)")
	flag.StringVar(&c.tlsCert, "tls-cert", "", "serve HTTPS with this certificate file (needs -tls-key)")
	flag.StringVar(&c.tlsKey, "tls-key", "", "private key for -tls-cert")
	flag.StringVar(&c.webDir, "web-dir", "", "serve the browser build from this directory (see `build web`)")
	flag.IntVar(&c.apiRate, "api-rate", 600, "requests per minute allowed per client IP on /api (0 disables)")
	flag.StringVar(&c.resetPassword, "reset-password", "", "give this user a temporary password (printed) that must be changed at their next login, sign out their devices, and exit")
	flag.Parse()
	return c
}

func main() {
	cfg := parseFlags()
	if cfg.resetPassword != "" {
		if err := printTemporaryPassword(cfg); err != nil {
			log.Fatal(err)
		}
		return
	}
	s, err := newServer(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer s.db.Close()
	log.Fatal(serve(cfg, s))
}

func newServer(cfg config) (*server, error) {
	db, err := openDB(cfg.dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if err := os.MkdirAll(cfg.mediaDir, 0o755); err != nil {
		return nil, fmt.Errorf("media dir: %w", err)
	}
	s := &server{
		db: db, hub: newHub(), calls: newCallRooms(), mediaDir: cfg.mediaDir, trustProxy: cfg.trustProxy,
		limiter:       newLimiter(10, time.Minute),
		uploadLimiter: newLimiter(30, time.Minute),
		origins:       allowedOrigins(cfg.origins, cfg.dev),
	}
	if cfg.apiRate > 0 {
		s.apiLimiter = newLimiter(cfg.apiRate, time.Minute)
	}
	if cfg.dev {
		if err := s.seedIfEmpty(); err != nil {
			return nil, fmt.Errorf("seed demo data: %w", err)
		}
		log.Printf("dev mode: browser origins allowed: %s", describeOrigins(s.origins))
	}
	presenceHook = s.markPresence
	s.hub.decorate = s.markPresence
	s.upgrader = websocket.Upgrader{ReadBufferSize: 4096, WriteBufferSize: 4096, CheckOrigin: s.originAllowed}
	go s.purgeSessions()
	return s, nil
}

// allowedOrigins parses -allowed-origins; development allows any origin
// unless a list was given (e.g. the web app served by Metro on another port).
func allowedOrigins(list string, dev bool) map[string]bool {
	origins := map[string]bool{}
	for _, o := range strings.Split(list, ",") {
		if o = strings.TrimSpace(o); o != "" {
			origins[o] = true
		}
	}
	if dev && len(origins) == 0 {
		origins["*"] = true
	}
	return origins
}

func describeOrigins(origins map[string]bool) string {
	names := make([]string, 0, len(origins))
	for o := range origins {
		names = append(names, o)
	}
	return strings.Join(names, ", ")
}

// handler is the whole HTTP surface: the API routes, the browser build when
// one is configured, and the middleware around them.
func (s *server) handler(webDir string) (http.Handler, error) {
	mux := http.NewServeMux()
	s.routes(mux)
	if webDir != "" {
		if _, err := os.Stat(filepath.Join(webDir, "index.html")); err != nil {
			return nil, fmt.Errorf("web dir: %v (build it with `build web`)", err)
		}
		mux.Handle("/", webApp(webDir))
	}
	return s.secureHeaders(s.cors(logRequests(s.apiLimit(mux)))), nil
}

// serve runs the HTTP server until it fails.
func serve(cfg config, s *server) error {
	h, err := s.handler(cfg.webDir)
	if err != nil {
		return err
	}
	srv := &http.Server{
		Addr:              cfg.addr,
		Handler:           h,
		MaxHeaderBytes:    64 << 10,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       2 * time.Minute,
		TLSConfig:         &tls.Config{MinVersion: tls.VersionTLS12},
	}
	log.Printf("ami listening on %s (db %s)", cfg.addr, cfg.dbPath)
	if cfg.tlsCert != "" || cfg.tlsKey != "" {
		return srv.ListenAndServeTLS(cfg.tlsCert, cfg.tlsKey)
	}
	return srv.ListenAndServe()
}

func (s *server) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, struct {
			Status string `json:"status"`
		}{"ok"})
	})

	// auth and profile
	mux.HandleFunc("POST /api/register", s.throttle(public(s.register)))
	mux.HandleFunc("POST /api/login", s.throttle(public(s.login)))
	mux.HandleFunc("POST /api/logout", s.auth(s.logout))
	mux.HandleFunc("GET /api/me", s.auth(s.me))
	mux.HandleFunc("PUT /api/me", s.auth(s.updateMe))
	mux.HandleFunc("PUT /api/me/password", s.auth(s.changePassword))
	mux.HandleFunc("PUT /api/me/presence", s.auth(s.setPresence))

	// users
	mux.HandleFunc("GET /api/users", s.auth(s.searchUsers))
	mux.HandleFunc("GET /api/users/{id}", s.auth(s.getUser))
	mux.HandleFunc("GET /api/users/{id}/posts", s.auth(s.userPosts))

	// posts, comments, reactions, media
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

	// groups and their chats
	mux.HandleFunc("GET /api/groups", s.auth(s.listGroups))
	mux.HandleFunc("POST /api/groups", s.auth(s.createGroup))
	mux.HandleFunc("GET /api/groups/{id}", s.auth(s.getGroupHandler))
	mux.HandleFunc("DELETE /api/groups/{id}", s.auth(s.deleteGroup))
	mux.HandleFunc("POST /api/groups/{id}/join", s.auth(s.joinGroup))
	mux.HandleFunc("POST /api/groups/{id}/leave", s.auth(s.leaveGroup))
	mux.HandleFunc("GET /api/groups/{id}/posts", s.auth(s.groupPosts))
	mux.HandleFunc("POST /api/groups/{id}/members", s.auth(s.addGroupMembers))
	mux.HandleFunc("DELETE /api/groups/{id}/members/{uid}", s.auth(s.removeGroupMember))
	mux.HandleFunc("GET /api/groups/{id}/messages", s.auth(s.groupChat(s.listChat)))
	mux.HandleFunc("POST /api/groups/{id}/messages", s.auth(s.groupChat(s.sendChat)))
	mux.HandleFunc("POST /api/groups/{id}/messages/read", s.auth(s.groupChat(s.readChat)))
	mux.HandleFunc("PUT /api/groups/{id}/messages/{mid}", s.auth(s.groupChat(s.updateChat)))
	mux.HandleFunc("DELETE /api/groups/{id}/messages/{mid}", s.auth(s.groupChat(s.deleteChat)))

	// direct messages and notifications
	mux.HandleFunc("GET /api/conversations", s.auth(s.listConversations))
	mux.HandleFunc("GET /api/messages/{id}", s.auth(s.directChat(s.listChat)))
	mux.HandleFunc("POST /api/messages/{id}", s.auth(s.directChat(s.sendChat)))
	mux.HandleFunc("POST /api/messages/{id}/read", s.auth(s.directChat(s.readChat)))
	mux.HandleFunc("PUT /api/messages/{id}/{mid}", s.auth(s.directChat(s.updateChat)))
	mux.HandleFunc("DELETE /api/messages/{id}/{mid}", s.auth(s.directChat(s.deleteChat)))
	mux.HandleFunc("PUT /api/push-token", s.auth(s.setPushToken))
	mux.HandleFunc("DELETE /api/push-token", s.auth(s.deletePushToken))

	// realtime: events + call signaling
	mux.HandleFunc("GET /ws", s.handleWS)
}

// ---- middleware

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
		origin := r.Header.Get("Origin")
		switch {
		case origin == "":
		case !s.originAllowed(r):
			if r.Method == http.MethodOptions {
				writeError(w, http.StatusForbidden, "origin not allowed")
				return
			}
		default:
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			h.Set("Access-Control-Max-Age", "600")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
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
