// Command server is the Ami backend: a single binary over MariaDB,
// JSON over HTTP, and a WebSocket endpoint (gorilla/websocket) for realtime events
// and WebRTC signaling.
//
// Production is the default: no CORS, rate-limited auth endpoints, session
// tokens hashed at rest, strict timeouts. Development extras are opt-in (-dev).
package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
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
	cpu           cpuMeter   // this process's recent CPU use, for /api/stats
	ice           iceConfig  // the STUN and TURN servers handed to the apps for calls (ice.go)
	invite        string     // registration needs this code when set (-invite)
	storageCap    int64      // the most one account may upload in total, in bytes; 0 is no limit (-user-storage-gb)
}

// config is everything the command-line flags decide.
type config struct {
	addr, dsn, mediaDir, origins, tlsCert, tlsKey, webDir string
	stunURLs, turnURLs, turnSecret                        string // ICE servers for calls (ice.go)
	turnPort                                              int
	invite                                                string // registration needs this code when set
	userStorageGB                                         int    // the most one account may upload in total; 0 is no limit
	dev, trustProxy, verbose                              bool
	apiRate                                               int
	resetPassword                                         string // a username: print a temporary password for it and exit
}

func parseFlags() config {
	var c config
	flag.StringVar(&c.addr, "addr", ":8080", "listen address")
	flag.StringVar(&c.dsn, "db", envOr("AMI_DB", "ami:ami@tcp(127.0.0.1:3306)/ami"),
		"MariaDB: user:password@tcp(host:3306)/database or user:password@unix(/path/mysql.sock)/database (default $AMI_DB)")
	flag.StringVar(&c.mediaDir, "media-dir", "media", "directory for uploaded pictures and videos")
	flag.BoolVar(&c.dev, "dev", false, "development mode: seed demo data into an empty database and allow any CORS origin")
	flag.StringVar(&c.origins, "allowed-origins", "", "comma-separated browser origins allowed to call the API (CORS); empty disables CORS")
	flag.BoolVar(&c.trustProxy, "trust-proxy", false, "take client IPs from X-Forwarded-For (only behind a trusted reverse proxy)")
	flag.StringVar(&c.tlsCert, "tls-cert", "", "serve HTTPS with this certificate file (needs -tls-key)")
	flag.StringVar(&c.tlsKey, "tls-key", "", "private key for -tls-cert")
	flag.StringVar(&c.webDir, "web-dir", "", "serve the browser build from this directory (see `build web`)")
	flag.StringVar(&c.stunURLs, "stun-urls", envOr("AMI_STUN_URLS", "stun:stun.l.google.com:19302"), "comma-separated STUN servers the apps use for calls")
	flag.StringVar(&c.turnSecret, "turn-secret", envOr("AMI_TURN_SECRET", ""), "the static-auth-secret of a coturn server; the apps then get TURN credentials for it from /api/ice")
	flag.StringVar(&c.turnURLs, "turn-urls", envOr("AMI_TURN_URLS", ""), "comma-separated TURN servers (turn:host:3478?transport=udp, turns:host:5349); empty means coturn on this host at -turn-port")
	flag.IntVar(&c.turnPort, "turn-port", envInt("AMI_TURN_PORT", 3478), "port of the coturn on this host, when -turn-urls is empty")
	flag.StringVar(&c.invite, "invite", envOr("AMI_INVITE", ""), "registration needs this invite code; empty leaves registration open to anyone")
	flag.IntVar(&c.userStorageGB, "user-storage-gb", envInt("AMI_USER_STORAGE_GB", 0), "the most one account may upload in total, in GB (0 = no limit)")
	flag.IntVar(&c.apiRate, "api-rate", 600, "requests per minute allowed per client IP on /api (0 disables)")
	flag.BoolVar(&c.verbose, "verbose", false, "log every request and response with headers and body, and every WebSocket message (passwords and tokens redacted, bodies cut at 4 KB)")
	flag.StringVar(&c.resetPassword, "reset-password", "", "give this user a temporary password (printed) that must be changed at their next login, sign out their devices, and exit")
	flag.Parse()
	return c
}

func main() {
	cfg := parseFlags()
	verbose = cfg.verbose
	if cfg.resetPassword != "" {
		if err := printTemporaryPassword(cfg); err != nil {
			log.Fatal(err)
		}
		return
	}
	// Ctrl-C or SIGTERM (systemd, process-compose) ends ctx: in-flight
	// requests finish, connections close, background loops stop.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	s, err := newServer(ctx, cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer s.db.Close()
	if err := serve(ctx, cfg, s); err != nil {
		log.Fatal(err)
	}
	log.Print("ami stopped")
}

// newServer opens the database and starts the background work, which runs
// until ctx ends.
func newServer(ctx context.Context, cfg config) (*server, error) {
	db, err := openDB(cfg.dsn)
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
		ice:           newICEConfig(cfg),
		invite:        cfg.invite,
		storageCap:    int64(cfg.userStorageGB) * 1_000_000_000,
	}
	if s.ice.secret != "" {
		log.Printf("calls: handing out TURN credentials for %s", s.ice.describe())
	}
	if s.invite != "" {
		log.Print("registration: by invite code")
	}
	if s.storageCap > 0 {
		log.Printf("uploads: %s per account", formatBytes(s.storageCap))
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
	if err := s.recordOldUploads(); err != nil {
		return nil, fmt.Errorf("record uploads: %w", err)
	}
	presenceHook = s.markPresence
	s.hub.decorate = s.markPresence
	s.upgrader = websocket.Upgrader{ReadBufferSize: 4096, WriteBufferSize: 4096, CheckOrigin: s.originAllowed, Subprotocols: []string{"ami"}}
	go s.purgeSessions(ctx)
	go s.meterCPU(ctx)
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
	logged := logRequests
	if verbose {
		logged = logTraffic // the whole exchange instead of one line
	}
	return s.secureHeaders(s.cors(logged(s.apiLimit(mux)))), nil
}

// shutdownGrace is how long in-flight requests get to finish on shutdown.
const shutdownGrace = 10 * time.Second

// serve runs the HTTP server until it fails or ctx ends; then it stops taking
// connections, lets in-flight requests finish and closes the WebSockets.
func serve(ctx context.Context, cfg config, s *server) error {
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
	failed := make(chan error, 1)
	go func() {
		log.Printf("ami listening on %s (db %s)", cfg.addr, redactDSN(cfg.dsn))
		if verbose {
			log.Print("verbose: logging headers, bodies and WebSocket traffic; passwords and tokens are redacted")
		}
		if cfg.tlsCert != "" || cfg.tlsKey != "" {
			failed <- srv.ListenAndServeTLS(cfg.tlsCert, cfg.tlsKey)
		} else {
			failed <- srv.ListenAndServe()
		}
	}()
	select {
	case err := <-failed:
		return err
	case <-ctx.Done():
	}
	log.Print("ami shutting down")
	grace, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()
	err = srv.Shutdown(grace)
	s.hub.closeAll() // Shutdown does not know about hijacked connections
	if errors.Is(err, context.DeadlineExceeded) {
		return nil // requests still running after the grace period are cut off; that is the deal
	}
	return err
}

func (s *server) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		registration := "open"
		if s.invite != "" {
			registration = "invite"
		}
		writeJSON(w, http.StatusOK, struct {
			Status       string `json:"status"`
			Registration string `json:"registration"` // "open", or "invite" when POST /api/register needs the code
		}{"ok", registration})
	})

	// auth and profile
	mux.HandleFunc("POST /api/register", s.throttle(public(s.register)))
	mux.HandleFunc("POST /api/login", s.throttle(public(s.login)))
	mux.HandleFunc("POST /api/logout", s.auth(s.logout))
	mux.HandleFunc("GET /api/me", s.auth(s.me))
	mux.HandleFunc("PUT /api/me", s.auth(s.updateMe))
	mux.HandleFunc("PUT /api/me/password", s.auth(s.changePassword))
	mux.HandleFunc("PUT /api/me/presence", s.auth(s.setPresence))
	mux.HandleFunc("GET /api/stats", s.auth(s.stats))
	mux.HandleFunc("GET /api/ice", s.auth(s.iceServers))

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
	mux.HandleFunc("GET /api/posts/{id}/reactions", s.auth(s.listReactions))
	mux.HandleFunc("GET /api/media/{name}/reactions", s.auth(s.listMediaReactions))
	mux.HandleFunc("PUT /api/media/{name}/reaction", s.auth(s.reactToMedia))
	mux.HandleFunc("DELETE /api/media/{name}/reaction", s.auth(s.unreactToMedia))
	mux.HandleFunc("GET /api/comments/{id}/reactions", s.auth(s.listCommentReactions))
	mux.HandleFunc("PUT /api/comments/{id}/reaction", s.auth(s.setCommentReaction))
	mux.HandleFunc("DELETE /api/comments/{id}/reaction", s.auth(s.deleteCommentReaction))
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
	mux.HandleFunc("GET /api/groups/{id}/messages/{mid}/reactions", s.auth(s.groupChat(s.listChatReactions)))
	mux.HandleFunc("PUT /api/groups/{id}/messages/{mid}/reaction", s.auth(s.groupChat(s.reactToChat)))
	mux.HandleFunc("DELETE /api/groups/{id}/messages/{mid}/reaction", s.auth(s.groupChat(s.unreactToChat)))

	// direct messages and notifications
	mux.HandleFunc("GET /api/conversations", s.auth(s.listConversations))
	mux.HandleFunc("GET /api/messages/{id}", s.auth(s.directChat(s.listChat)))
	mux.HandleFunc("POST /api/messages/{id}", s.auth(s.directChat(s.sendChat)))
	mux.HandleFunc("POST /api/messages/{id}/read", s.auth(s.directChat(s.readChat)))
	mux.HandleFunc("PUT /api/messages/{id}/{mid}", s.auth(s.directChat(s.updateChat)))
	mux.HandleFunc("DELETE /api/messages/{id}/{mid}", s.auth(s.directChat(s.deleteChat)))
	mux.HandleFunc("GET /api/messages/{id}/{mid}/reactions", s.auth(s.directChat(s.listChatReactions)))
	mux.HandleFunc("PUT /api/messages/{id}/{mid}/reaction", s.auth(s.directChat(s.reactToChat)))
	mux.HandleFunc("DELETE /api/messages/{id}/{mid}/reaction", s.auth(s.directChat(s.unreactToChat)))
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
