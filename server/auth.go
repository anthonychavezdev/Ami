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
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
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
	AvatarURL   string `json:"avatar_url,omitempty"` // an uploaded picture under /media, or empty for initials
	Pronouns    string `json:"pronouns,omitempty"`
	Online      bool   `json:"online"` // has a live connection; filled in by markPresence on output
}

// Me is the caller's own user with private settings and the full profile.
type Me struct {
	User
	Invisible bool   `json:"invisible"` // appear offline to everyone
	Bio       string `json:"bio"`
}

func (s *server) meFor(u User) Me {
	m := Me{User: u}
	_ = s.db.QueryRow(`SELECT invisible, bio FROM users WHERE id = ?`, u.ID).Scan(&m.Invisible, &m.Bio)
	return m
}

// Names nobody may take: "deleted" is the placeholder author of removed comments.
var reservedUsernames = map[string]bool{"deleted": true, "admin": true, "ami": true, "me": true, "system": true}

var avatarExts = map[string]bool{".jpg": true, ".png": true, ".gif": true, ".webp": true}

// updateMe changes the caller's profile; fields left out are kept. A new
// username must be valid, not reserved and not anyone else's (any case).
func (s *server) updateMe(w http.ResponseWriter, r *http.Request, me *User) {
	var in struct {
		Username    *string `json:"username"`
		DisplayName *string `json:"display_name"`
		Bio         *string `json:"bio"`
		Pronouns    *string `json:"pronouns"`
		AvatarURL   *string `json:"avatar_url"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	var sets []string
	var args []any
	if in.Username != nil {
		u := strings.TrimSpace(*in.Username)
		if !usernameRe.MatchString(u) {
			writeError(w, http.StatusBadRequest, "username must be 3-24 letters, digits or underscores")
			return
		}
		if reservedUsernames[strings.ToLower(u)] {
			writeError(w, http.StatusBadRequest, "that username is reserved")
			return
		}
		var other int64
		err := s.db.QueryRow(`SELECT id FROM users WHERE username = ? COLLATE NOCASE AND id != ?`, u, me.ID).Scan(&other)
		if err == nil {
			writeError(w, http.StatusConflict, "username already taken")
			return
		}
		if !errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		sets, args = append(sets, "username = ?"), append(args, u)
	}
	if in.DisplayName != nil {
		d := strings.TrimSpace(*in.DisplayName)
		if n := utf8.RuneCountInString(d); n == 0 || n > 50 {
			writeError(w, http.StatusBadRequest, "display name must be 1-50 characters")
			return
		}
		sets, args = append(sets, "display_name = ?"), append(args, d)
	}
	if in.Bio != nil {
		b := strings.TrimSpace(*in.Bio)
		if utf8.RuneCountInString(b) > 500 {
			writeError(w, http.StatusBadRequest, "bio must be at most 500 characters")
			return
		}
		sets, args = append(sets, "bio = ?"), append(args, b)
	}
	if in.Pronouns != nil {
		pr := strings.TrimSpace(*in.Pronouns)
		if utf8.RuneCountInString(pr) > 30 {
			writeError(w, http.StatusBadRequest, "pronouns must be at most 30 characters")
			return
		}
		sets, args = append(sets, "pronouns = ?"), append(args, pr)
	}
	if in.AvatarURL != nil {
		a := strings.TrimSpace(*in.AvatarURL)
		if a != "" {
			name := strings.TrimPrefix(a, "/media/")
			if name == a || name == "" || strings.ContainsAny(name, `/\`) || !avatarExts[strings.ToLower(filepath.Ext(name))] {
				writeError(w, http.StatusBadRequest, "profile pictures must be pictures uploaded via /api/media")
				return
			}
			if _, err := os.Stat(filepath.Join(s.mediaDir, name)); err != nil {
				writeError(w, http.StatusBadRequest, "unknown media file "+name)
				return
			}
		}
		sets, args = append(sets, "avatar_url = ?"), append(args, a)
	}
	if len(sets) == 0 {
		writeError(w, http.StatusBadRequest, "nothing to update")
		return
	}
	args = append(args, me.ID)
	if _, err := s.db.Exec(`UPDATE users SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") { // lost a race for the name
			writeError(w, http.StatusConflict, "username already taken")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	u, err := s.userByID(me.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.meFor(u))
}

func (s *server) invisible(userID int64) bool {
	var v bool
	_ = s.db.QueryRow(`SELECT invisible FROM users WHERE id = ?`, userID).Scan(&v)
	return v
}

// setPresence lets a user appear offline (or visible again).
func (s *server) setPresence(w http.ResponseWriter, r *http.Request, me *User) {
	var in struct {
		Invisible bool `json:"invisible"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if _, err := s.db.Exec(`UPDATE users SET invisible = ? WHERE id = ?`, in.Invisible, me.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.hub.setHidden(me.ID, in.Invisible)
	writeJSON(w, http.StatusOK, map[string]bool{"invisible": in.Invisible})
}

const userCols = `u.id, u.username, u.display_name, u.created_at, u.avatar_url, u.pronouns`

func scanUser(sc interface{ Scan(...any) error }) (User, error) {
	var u User
	err := sc.Scan(&u.ID, &u.Username, &u.DisplayName, &u.CreatedAt, &u.AvatarURL, &u.Pronouns)
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
	if err := row.Scan(&u.ID, &u.Username, &u.DisplayName, &u.CreatedAt, &u.AvatarURL, &u.Pronouns, &expires); err != nil {
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
	writeJSON(w, http.StatusOK, map[string]any{"token": token, "user": s.meFor(u)})
}

func (s *server) logout(w http.ResponseWriter, r *http.Request, me *User) {
	_, _ = s.db.Exec(`DELETE FROM sessions WHERE token = ?`, tokenHash(bearerToken(r)))
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) me(w http.ResponseWriter, r *http.Request, me *User) {
	writeJSON(w, http.StatusOK, s.meFor(*me))
}

func (s *server) userByID(id int64) (User, error) {
	return scanUser(s.db.QueryRow(`SELECT `+userCols+` FROM users u WHERE u.id = ?`, id))
}
