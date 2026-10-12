package main

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// Accounts and sessions: passwords are PBKDF2-HMAC-SHA256 with a per-user
// salt; a session is a random token the client keeps, stored hashed.

// pbkdf2Iterations follows OWASP's 2023 guidance; it is stored per user and
// older hashes are upgraded on login. A variable so tests can lower it.
var pbkdf2Iterations = 600_000

const sessionTTL = 30 * 24 * time.Hour

var usernameRe = regexp.MustCompile(`^[A-Za-z0-9_]{3,24}$`)

// ---- passwords and tokens

func hashPassword(password string, salt []byte, iterations int) []byte {
	key, err := pbkdf2.Key(sha256.New, password, salt, iterations, 32)
	if err != nil {
		panic(err) // only fails under FIPS constraints we never hit
	}
	return key
}

func randomBytes(n int) []byte {
	b := make([]byte, n)
	rand.Read(b) // never fails in Go 1.24+
	return b
}

// tokenHash is what gets stored: a leaked database must not yield live sessions.
func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func bearerToken(r *http.Request) string {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(h[len("Bearer "):])
	}
	return r.URL.Query().Get("token")
}

func (s *server) userForToken(token string) (*User, error) {
	if token == "" {
		return nil, errUnauthorized
	}
	row := s.db.QueryRow(`SELECT `+userCols+`, s.expires_at FROM sessions s JOIN users u ON u.id = s.user_id WHERE s.token = ?`, tokenHash(token))
	var u User
	var expires string
	if err := row.Scan(fields(u.fields(), []any{&expires})...); err != nil {
		return nil, errUnauthorized
	}
	if exp, err := time.Parse(time.RFC3339Nano, expires); err != nil || exp.Before(now()) {
		return nil, errUnauthorized
	}
	return &u, nil
}

func (s *server) createSession(userID int64) (string, error) {
	token := hex.EncodeToString(randomBytes(32))
	_, err := s.db.Exec(`INSERT INTO sessions(token, user_id, expires_at) VALUES (?, ?, ?)`,
		tokenHash(token), userID, now().Add(sessionTTL).Format(time.RFC3339Nano))
	return token, err
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

type authedHandler func(w http.ResponseWriter, r *http.Request, me *User) error

// auth wraps a handler, requiring a valid session token
// (Authorization: Bearer <token>, or ?token=<token> for WebSockets).
func (s *server) auth(h authedHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		me, err := s.userForToken(bearerToken(r))
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		respond(w, h(w, r, me))
	}
}

// ---- register, login, logout

type credentials struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Password    string `json:"password"`
}

// validCredentials trims and checks a registration.
func validCredentials(c credentials) (credentials, error) {
	c.Username, c.DisplayName = strings.TrimSpace(c.Username), strings.TrimSpace(c.DisplayName)
	switch {
	case !usernameRe.MatchString(c.Username):
		return c, badRequest("username must be 3-24 letters, digits or underscores")
	case len(c.Password) < 8 || len(c.Password) > 128:
		return c, badRequest("password must be 8-128 characters")
	case len(c.DisplayName) > 50:
		return c, badRequest("display name too long")
	}
	if c.DisplayName == "" {
		c.DisplayName = c.Username
	}
	return c, nil
}

func (s *server) register(w http.ResponseWriter, r *http.Request) error {
	var in credentials
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	c, err := validCredentials(in)
	if err != nil {
		return err
	}
	salt := randomBytes(16)
	res, err := s.db.Exec(`INSERT INTO users(username, display_name, password_hash, salt, password_iterations, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		c.Username, c.DisplayName, hashPassword(c.Password, salt, pbkdf2Iterations), salt, pbkdf2Iterations, nowString())
	if isUniqueViolation(err) {
		return conflict("username already taken")
	}
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	return s.issueSession(w, id)
}

// storedPassword is what the users table keeps for a login check.
type storedPassword struct {
	id         int64
	hash, salt []byte
	iterations int
}

func (p storedPassword) matches(password string) bool {
	return subtle.ConstantTimeCompare(p.hash, hashPassword(password, p.salt, p.iterations)) == 1
}

func (s *server) login(w http.ResponseWriter, r *http.Request) error {
	var c credentials
	if err := readJSON(w, r, &c); err != nil {
		return err
	}
	var p storedPassword
	err := s.db.QueryRow(`SELECT id, password_hash, salt, password_iterations FROM users WHERE username = ?`, strings.TrimSpace(c.Username)).
		Scan(&p.id, &p.hash, &p.salt, &p.iterations)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !p.matches(c.Password)) {
		return fail(http.StatusUnauthorized, "invalid username or password")
	}
	if err != nil {
		return err
	}
	if p.iterations < pbkdf2Iterations {
		s.upgradeHash(p.id, c.Password)
	}
	return s.issueSession(w, p.id)
}

// upgradeHash transparently re-hashes a password stored with fewer rounds.
func (s *server) upgradeHash(id int64, password string) {
	salt := randomBytes(16)
	if _, err := s.db.Exec(`UPDATE users SET password_hash = ?, salt = ?, password_iterations = ? WHERE id = ?`,
		hashPassword(password, salt, pbkdf2Iterations), salt, pbkdf2Iterations, id); err != nil {
		log.Printf("upgrade password hash: %v", err)
	}
}

// session is the answer of /api/register and /api/login.
type session struct {
	Token string `json:"token"`
	User  Me     `json:"user"`
}

func (s *server) issueSession(w http.ResponseWriter, userID int64) error {
	token, err := s.createSession(userID)
	if err != nil {
		return err
	}
	u, err := s.userByID(userID)
	if err != nil {
		return err
	}
	return ok(w, session{Token: token, User: s.meFor(u)})
}

func (s *server) logout(w http.ResponseWriter, r *http.Request, me *User) error {
	_, _ = s.db.Exec(`DELETE FROM sessions WHERE token = ?`, tokenHash(bearerToken(r)))
	return noContent(w)
}

func (s *server) me(w http.ResponseWriter, r *http.Request, me *User) error {
	return ok(w, s.meFor(*me))
}
