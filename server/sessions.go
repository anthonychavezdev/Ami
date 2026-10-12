package main

import (
	"context"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
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

// sessionUser is the account behind a token, and whether it is restricted to
// setting a new password.
type sessionUser struct {
	*User
	mustChangePassword bool
}

func (s *server) userForToken(token string) (sessionUser, error) {
	if token == "" {
		return sessionUser{}, errUnauthorized
	}
	row := s.db.QueryRow(`SELECT `+userCols+`, u.must_change_password, s.expires_at FROM sessions s JOIN users u ON u.id = s.user_id WHERE s.token = ?`, tokenHash(token))
	var u User
	var mustChange bool
	var expires string
	if err := row.Scan(fields(u.fields(), []any{&mustChange, &expires})...); err != nil {
		return sessionUser{}, errUnauthorized
	}
	if exp, err := time.Parse(time.RFC3339Nano, expires); err != nil || exp.Before(now()) {
		return sessionUser{}, errUnauthorized
	}
	return sessionUser{User: &u, mustChangePassword: mustChange}, nil
}

func (s *server) createSession(userID int64) (string, error) {
	token := hex.EncodeToString(randomBytes(32))
	_, err := s.db.Exec(`INSERT INTO sessions(token, user_id, expires_at) VALUES (?, ?, ?)`,
		tokenHash(token), userID, now().Add(sessionTTL).Format(time.RFC3339Nano))
	return token, err
}

// purgeSessions drops expired sessions at start and then hourly, until ctx ends.
func (s *server) purgeSessions(ctx context.Context) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		if _, err := s.db.Exec(`DELETE FROM sessions WHERE expires_at < ?`, nowString()); err != nil {
			log.Printf("purge expired sessions: %v", err)
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return
		}
	}
}

type authedHandler func(w http.ResponseWriter, r *http.Request, me *User) error

// passwordChangeOnly is all a user signed in with a temporary password may do.
var passwordChangeOnly = map[string]bool{"GET /api/me": true, "PUT /api/me/password": true, "POST /api/logout": true}

// auth wraps a handler, requiring a valid session token
// (Authorization: Bearer <token>, or ?token=<token> for WebSockets).
func (s *server) auth(h authedHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		su, err := s.userForToken(bearerToken(r))
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		if su.mustChangePassword && !passwordChangeOnly[r.Method+" "+r.URL.Path] {
			writeError(w, http.StatusForbidden, "set a new password first")
			return
		}
		respond(w, h(w, r, su.User))
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
	case validPassword(c.Password) != nil:
		return c, validPassword(c.Password)
	case len(c.DisplayName) > 50:
		return c, badRequest("display name too long")
	}
	if c.DisplayName == "" {
		c.DisplayName = c.Username
	}
	return c, nil
}

func validPassword(p string) error {
	if len(p) < 8 || len(p) > 128 {
		return badRequest("password must be 8-128 characters")
	}
	return nil
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

// passwordOf loads a user's stored password by username or id.
func (s *server) passwordOf(column string, key any) (storedPassword, error) {
	var p storedPassword
	err := s.db.QueryRow(`SELECT id, password_hash, salt, password_iterations FROM users WHERE `+column+` = ?`, key).
		Scan(&p.id, &p.hash, &p.salt, &p.iterations)
	return p, err
}

func (s *server) login(w http.ResponseWriter, r *http.Request) error {
	var c credentials
	if err := readJSON(w, r, &c); err != nil {
		return err
	}
	p, err := s.passwordOf("username", strings.TrimSpace(c.Username))
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !p.matches(c.Password)) {
		return fail(http.StatusUnauthorized, "invalid username or password")
	}
	if err != nil {
		return err
	}
	if p.iterations < pbkdf2Iterations { // transparently re-hash a password stored with fewer rounds
		if err := storePassword(s.db, p.id, c.Password); err != nil {
			log.Printf("upgrade password hash: %v", err)
		}
	}
	return s.issueSession(w, p.id)
}

// storePassword hashes a password with a fresh salt and the current rounds.
func storePassword(db dbtx, userID int64, password string) error {
	salt := randomBytes(16)
	_, err := db.Exec(`UPDATE users SET password_hash = ?, salt = ?, password_iterations = ? WHERE id = ?`,
		hashPassword(password, salt, pbkdf2Iterations), salt, pbkdf2Iterations, userID)
	return err
}

// ---- changing and resetting passwords

type passwordInput struct {
	Current string `json:"current"`
	New     string `json:"new"`
}

// changePassword sets a new password for the caller, who must give the
// current (or temporary) one, and signs out their other devices.
func (s *server) changePassword(w http.ResponseWriter, r *http.Request, me *User) error {
	var in passwordInput
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	if err := validPassword(in.New); err != nil {
		return err
	}
	p, err := s.passwordOf("id", me.ID)
	if err != nil {
		return err
	}
	if !p.matches(in.Current) {
		return badRequest("current password is wrong")
	}
	err = s.inTx(func(tx *sql.Tx) error {
		if err := storePassword(tx, me.ID, in.New); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE users SET must_change_password = 0 WHERE id = ?`, me.ID); err != nil {
			return err
		}
		_, err := tx.Exec(`DELETE FROM sessions WHERE user_id = ? AND token != ?`, me.ID, tokenHash(bearerToken(r)))
		return err
	})
	if err != nil {
		return err
	}
	return ok(w, s.meFor(*me))
}

// resetPassword (the -reset-password flag) gives an account a temporary
// password to pass on to its owner in person, and signs out every device.
// Logging in with it allows nothing but setting a new password.
func resetPassword(db *sql.DB, username string) (string, error) {
	var id int64
	err := db.QueryRow(`SELECT id FROM users WHERE username = ?`, strings.TrimSpace(username)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("no user named %q", username)
	}
	if err != nil {
		return "", err
	}
	password := temporaryPassword()
	err = inTx(db, func(tx *sql.Tx) error {
		if err := storePassword(tx, id, password); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE users SET must_change_password = 1 WHERE id = ?`, id); err != nil {
			return err
		}
		_, err := tx.Exec(`DELETE FROM sessions WHERE user_id = ?`, id)
		return err
	})
	return password, err
}

// temporaryPassword is a random passphrase that is easy to read out: three
// groups of four lowercase letters and digits, without look-alikes.
func temporaryPassword() string {
	const alphabet = "abcdefghjkmnpqrstuvwxyz023456789" // 32 characters: no i, l, o or 1
	out := make([]byte, 0, 14)
	for i, b := range randomBytes(12) {
		if i > 0 && i%4 == 0 {
			out = append(out, '-')
		}
		out = append(out, alphabet[b%32])
	}
	return string(out)
}

// printTemporaryPassword runs the -reset-password flag and exits the program.
func printTemporaryPassword(cfg config) error {
	db, err := openDB(cfg.dbPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()
	password, err := resetPassword(db, cfg.resetPassword)
	if err != nil {
		return err
	}
	fmt.Printf("Temporary password for %s: %s\nIt only allows setting a new password at the next login; every device of this user has been signed out.\n", cfg.resetPassword, password)
	return nil
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
