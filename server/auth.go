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

const (
	pbkdf2Iterations = 600_000 // OWASP 2023 guidance for PBKDF2-HMAC-SHA256; stored per user, older hashes are upgraded on login
	sessionTTL       = 30 * 24 * time.Hour
)

var usernameRe = regexp.MustCompile(`^[A-Za-z0-9_]{3,24}$`)

type User struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	CreatedAt   string `json:"created_at"`
}

const userCols = `u.id, u.username, u.display_name, u.created_at`

func scanUser(sc interface{ Scan(...any) error }) (User, error) {
	var u User
	err := sc.Scan(&u.ID, &u.Username, &u.DisplayName, &u.CreatedAt)
	return u, err
}

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

type authedHandler func(w http.ResponseWriter, r *http.Request, me *User)

// auth wraps a handler, requiring a valid session token
// (Authorization: Bearer <token>, or ?token=<token> for WebSockets).
func (s *server) auth(h authedHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		me, err := s.userForToken(bearerToken(r))
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		h(w, r, me)
	}
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
	if err := row.Scan(&u.ID, &u.Username, &u.DisplayName, &u.CreatedAt, &expires); err != nil {
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

type credentials struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Password    string `json:"password"`
}

func (s *server) register(w http.ResponseWriter, r *http.Request) {
	var c credentials
	if !readJSON(w, r, &c) {
		return
	}
	c.Username = strings.TrimSpace(c.Username)
	c.DisplayName = strings.TrimSpace(c.DisplayName)
	if !usernameRe.MatchString(c.Username) {
		writeError(w, http.StatusBadRequest, "username must be 3-24 letters, digits or underscores")
		return
	}
	if len(c.Password) < 8 || len(c.Password) > 128 {
		writeError(w, http.StatusBadRequest, "password must be 8-128 characters")
		return
	}
	if c.DisplayName == "" {
		c.DisplayName = c.Username
	}
	if len(c.DisplayName) > 50 {
		writeError(w, http.StatusBadRequest, "display name too long")
		return
	}
	salt := randomBytes(16)
	res, err := s.db.Exec(`INSERT INTO users(username, display_name, password_hash, salt, password_iterations, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		c.Username, c.DisplayName, hashPassword(c.Password, salt, pbkdf2Iterations), salt, pbkdf2Iterations, nowString())
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeError(w, http.StatusConflict, "username already taken")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	id, _ := res.LastInsertId()
	s.issueSession(w, id)
}

func (s *server) login(w http.ResponseWriter, r *http.Request) {
	var c credentials
	if !readJSON(w, r, &c) {
		return
	}
	var id int64
	var hash, salt []byte
	var iters int
	err := s.db.QueryRow(`SELECT id, password_hash, salt, password_iterations FROM users WHERE username = ?`, strings.TrimSpace(c.Username)).Scan(&id, &hash, &salt, &iters)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && subtle.ConstantTimeCompare(hash, hashPassword(c.Password, salt, iters)) != 1) {
		writeError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if iters < pbkdf2Iterations { // transparently upgrade hashes made with fewer rounds
		salt = randomBytes(16)
		if _, err := s.db.Exec(`UPDATE users SET password_hash = ?, salt = ?, password_iterations = ? WHERE id = ?`, hashPassword(c.Password, salt, pbkdf2Iterations), salt, pbkdf2Iterations, id); err != nil {
			log.Printf("upgrade password hash: %v", err)
		}
	}
	s.issueSession(w, id)
}

func (s *server) issueSession(w http.ResponseWriter, userID int64) {
	token, err := s.createSession(userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	u, err := s.userByID(userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": token, "user": u})
}

func (s *server) logout(w http.ResponseWriter, r *http.Request, me *User) {
	_, _ = s.db.Exec(`DELETE FROM sessions WHERE token = ?`, tokenHash(bearerToken(r)))
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) me(w http.ResponseWriter, r *http.Request, me *User) {
	writeJSON(w, http.StatusOK, me)
}

func (s *server) userByID(id int64) (User, error) {
	return scanUser(s.db.QueryRow(`SELECT `+userCols+` FROM users u WHERE u.id = ?`, id))
}
