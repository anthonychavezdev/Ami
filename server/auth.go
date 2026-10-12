package main

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image"
	"log"
	"math"
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
	ID          int64      `json:"id"`
	Username    string     `json:"username"`
	DisplayName string     `json:"display_name"`
	CreatedAt   string     `json:"created_at"`
	AvatarURL   string     `json:"avatar_url,omitempty"` // an uploaded picture under /media, or empty for initials
	AvatarCrop  AvatarCrop `json:"avatar_crop"`          // the part of the picture shown in the circle (W == 0: whole picture, centred)
	Pronouns    string     `json:"pronouns,omitempty"`
	Online      bool       `json:"online"` // has a live connection; filled in by markPresence on output
}

// AvatarCrop is a square window on the profile picture: X and Y are the
// top-left corner as fractions of the width and height, W the side as a
// fraction of the width, and Aspect the picture's width/height ratio (the
// side as a fraction of the height is W*Aspect). Stored as JSON text.
type AvatarCrop struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	W      float64 `json:"w"`
	Aspect float64 `json:"aspect"`
}

func (c *AvatarCrop) Scan(src any) error {
	*c = AvatarCrop{}
	var raw string
	switch v := src.(type) {
	case string:
		raw = v
	case []byte:
		raw = string(v)
	}
	if raw == "" {
		return nil
	}
	return json.Unmarshal([]byte(raw), c)
}

// pictureAspect is width/height of an uploaded picture, or 0 when it cannot
// be decoded (WebP has no decoder in the standard library).
func (s *server) pictureAspect(url string) float64 {
	f, err := os.Open(filepath.Join(s.mediaDir, strings.TrimPrefix(url, "/media/")))
	if err != nil {
		return 0
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil || cfg.Height == 0 {
		return 0
	}
	return float64(cfg.Width) / float64(cfg.Height)
}

// defaultCrop is the largest centred square of a picture.
func defaultCrop(aspect float64) AvatarCrop {
	if aspect <= 0 {
		aspect = 1
	}
	w := math.Min(1, 1/aspect)
	return AvatarCrop{X: (1 - w) / 2, Y: (1 - w*aspect) / 2, W: w, Aspect: aspect}.rounded()
}

// rounded trims floating-point noise (e.g. 2.8e-17) before storing.
func (c AvatarCrop) rounded() AvatarCrop {
	r := func(v float64) float64 { return math.Round(v*10000) / 10000 }
	return AvatarCrop{X: r(c.X), Y: r(c.Y), W: r(c.W), Aspect: r(c.Aspect)}
}

func (c AvatarCrop) valid() bool {
	const eps = 0.001
	return c.W > 0 && c.W <= 1+eps && c.Aspect > 0.05 && c.Aspect < 20 && c.X >= -eps && c.Y >= -eps && c.X+c.W <= 1+eps && c.Y+c.W*c.Aspect <= 1+eps
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
		Username    *string     `json:"username"`
		DisplayName *string     `json:"display_name"`
		Bio         *string     `json:"bio"`
		Pronouns    *string     `json:"pronouns"`
		AvatarURL   *string     `json:"avatar_url"`
		AvatarCrop  *AvatarCrop `json:"avatar_crop"`
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
	avatarURL, newAvatar := "", false
	if in.AvatarURL != nil {
		a := strings.TrimSpace(*in.AvatarURL)
		avatarURL, newAvatar = a, a != ""
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
		if a == "" { // no picture, no crop
			sets, args = append(sets, "avatar_crop = ?"), append(args, "")
			in.AvatarCrop = nil
		}
	}
	// The picture decides its aspect ratio; a new picture without a crop gets
	// the centred square, so every avatar always has a usable crop.
	if newAvatar || in.AvatarCrop != nil {
		if !newAvatar {
			if err := s.db.QueryRow(`SELECT avatar_url FROM users WHERE id = ?`, me.ID).Scan(&avatarURL); err != nil || avatarURL == "" {
				writeError(w, http.StatusBadRequest, "no profile picture to position")
				return
			}
		}
		aspect := s.pictureAspect(avatarURL)
		crop := defaultCrop(aspect)
		if in.AvatarCrop != nil {
			crop = *in.AvatarCrop
			if aspect > 0 {
				crop.Aspect = aspect
			}
			if !crop.valid() {
				writeError(w, http.StatusBadRequest, "avatar_crop must be a square inside the picture: x, y, w in 0-1")
				return
			}
		}
		raw, _ := json.Marshal(crop.rounded())
		sets, args = append(sets, "avatar_crop = ?"), append(args, string(raw))
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

const userCols = `u.id, u.username, u.display_name, u.created_at, u.avatar_url, u.pronouns, u.avatar_crop`

func scanUser(sc interface{ Scan(...any) error }) (User, error) {
	var u User
	err := sc.Scan(&u.ID, &u.Username, &u.DisplayName, &u.CreatedAt, &u.AvatarURL, &u.Pronouns, &u.AvatarCrop)
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
	if err := row.Scan(&u.ID, &u.Username, &u.DisplayName, &u.CreatedAt, &u.AvatarURL, &u.Pronouns, &u.AvatarCrop, &expires); err != nil {
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
