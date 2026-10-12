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
	"fmt"
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

const userCols = `u.id, u.username, u.display_name, u.created_at, u.avatar_url, u.pronouns, u.avatar_crop`

func scanUser(sc scanner) (User, error) {
	var u User
	err := sc.Scan(&u.ID, &u.Username, &u.DisplayName, &u.CreatedAt, &u.AvatarURL, &u.Pronouns, &u.AvatarCrop)
	return u, err
}

func (s *server) userByID(id int64) (User, error) {
	return scanUser(s.db.QueryRow(`SELECT `+userCols+` FROM users u WHERE u.id = ?`, id))
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

// ---- profile pictures

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

// rounded trims floating-point noise (e.g. 2.8e-17) before storing.
func (c AvatarCrop) rounded() AvatarCrop {
	r := func(v float64) float64 { return math.Round(v*10000) / 10000 }
	return AvatarCrop{X: r(c.X), Y: r(c.Y), W: r(c.W), Aspect: r(c.Aspect)}
}

func (c AvatarCrop) valid() bool {
	const eps = 0.001
	return c.W > 0 && c.W <= 1+eps && c.Aspect > 0.05 && c.Aspect < 20 && c.X >= -eps && c.Y >= -eps && c.X+c.W <= 1+eps && c.Y+c.W*c.Aspect <= 1+eps
}

// defaultCrop is the largest centred square of a picture.
func defaultCrop(aspect float64) AvatarCrop {
	if aspect <= 0 {
		aspect = 1
	}
	w := math.Min(1, 1/aspect)
	return AvatarCrop{X: (1 - w) / 2, Y: (1 - w*aspect) / 2, W: w, Aspect: aspect}.rounded()
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

// ---- passwords and sessions

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

func (s *server) issueSession(w http.ResponseWriter, userID int64) error {
	token, err := s.createSession(userID)
	if err != nil {
		return err
	}
	u, err := s.userByID(userID)
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"token": token, "user": s.meFor(u)})
}

func (s *server) logout(w http.ResponseWriter, r *http.Request, me *User) error {
	_, _ = s.db.Exec(`DELETE FROM sessions WHERE token = ?`, tokenHash(bearerToken(r)))
	return noContent(w)
}

func (s *server) me(w http.ResponseWriter, r *http.Request, me *User) error {
	return ok(w, s.meFor(*me))
}

// ---- presence setting

func (s *server) invisible(userID int64) bool {
	var v bool
	_ = s.db.QueryRow(`SELECT invisible FROM users WHERE id = ?`, userID).Scan(&v)
	return v
}

// setPresence lets a user appear offline (or visible again).
func (s *server) setPresence(w http.ResponseWriter, r *http.Request, me *User) error {
	var in struct {
		Invisible bool `json:"invisible"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	if _, err := s.db.Exec(`UPDATE users SET invisible = ? WHERE id = ?`, in.Invisible, me.ID); err != nil {
		return err
	}
	s.hub.setHidden(me.ID, in.Invisible)
	return ok(w, map[string]bool{"invisible": in.Invisible})
}

// ---- profile changes

// Names nobody may take: "deleted" is the placeholder author of removed comments.
var reservedUsernames = map[string]bool{"deleted": true, "admin": true, "ami": true, "me": true, "system": true}

var avatarExts = map[string]bool{".jpg": true, ".png": true, ".gif": true, ".webp": true}

// profileInput is a PUT /api/me body: only the fields sent are changed.
type profileInput struct {
	Username    *string     `json:"username"`
	DisplayName *string     `json:"display_name"`
	Bio         *string     `json:"bio"`
	Pronouns    *string     `json:"pronouns"`
	AvatarURL   *string     `json:"avatar_url"`
	AvatarCrop  *AvatarCrop `json:"avatar_crop"`
}

// columnPatch is the SET part of an UPDATE, built one column at a time.
type columnPatch struct {
	sets []string
	args []any
}

func (p columnPatch) with(column string, v any) columnPatch {
	return columnPatch{sets: append(append([]string{}, p.sets...), column+" = ?"), args: append(append([]any{}, p.args...), v)}
}

// updateMe changes the caller's profile; fields left out are kept. A new
// username must be valid, not reserved and not anyone else's (any case).
func (s *server) updateMe(w http.ResponseWriter, r *http.Request, me *User) error {
	var in profileInput
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	p, err := s.profilePatch(me.ID, in)
	if err != nil {
		return err
	}
	if len(p.sets) == 0 {
		return badRequest("nothing to update")
	}
	_, err = s.db.Exec(`UPDATE users SET `+strings.Join(p.sets, ", ")+` WHERE id = ?`, append(p.args, me.ID)...)
	if isUniqueViolation(err) {
		return conflict("username already taken") // lost a race for the name
	}
	if err != nil {
		return err
	}
	u, err := s.userByID(me.ID)
	if err != nil {
		return err
	}
	return ok(w, s.meFor(u))
}

// profilePatch validates every field that was sent and turns it into a column change.
func (s *server) profilePatch(userID int64, in profileInput) (columnPatch, error) {
	var p columnPatch
	if in.Username != nil {
		u, err := s.newUsername(*in.Username, userID)
		if err != nil {
			return p, err
		}
		p = p.with("username", u)
	}
	if in.DisplayName != nil {
		d, err := validDisplayName(*in.DisplayName)
		if err != nil {
			return p, err
		}
		p = p.with("display_name", d)
	}
	if in.Bio != nil {
		b, err := shortText(*in.Bio, 500, "bio")
		if err != nil {
			return p, err
		}
		p = p.with("bio", b)
	}
	if in.Pronouns != nil {
		pr, err := shortText(*in.Pronouns, 30, "pronouns")
		if err != nil {
			return p, err
		}
		p = p.with("pronouns", pr)
	}
	return s.avatarPatch(p, userID, in)
}

// avatarPatch adds the picture and its crop. The picture decides its aspect
// ratio and a new picture without a crop gets the centred square, so every
// avatar always has a usable crop; no picture means no crop.
func (s *server) avatarPatch(p columnPatch, userID int64, in profileInput) (columnPatch, error) {
	url, changed := "", false
	if in.AvatarURL != nil {
		a, err := s.validAvatar(*in.AvatarURL)
		if err != nil {
			return p, err
		}
		if a == "" {
			return p.with("avatar_url", "").with("avatar_crop", ""), nil
		}
		url, changed = a, true
		p = p.with("avatar_url", a)
	}
	if !changed && in.AvatarCrop == nil {
		return p, nil
	}
	if !changed {
		var err error
		if url, err = s.currentAvatar(userID); err != nil {
			return p, err
		}
	}
	crop, err := s.cropFor(url, in.AvatarCrop)
	if err != nil {
		return p, err
	}
	return p.with("avatar_crop", jsonString(crop)), nil
}

// newUsername checks a username change: valid, not reserved, nobody else's.
func (s *server) newUsername(raw string, userID int64) (string, error) {
	u := strings.TrimSpace(raw)
	if !usernameRe.MatchString(u) {
		return "", badRequest("username must be 3-24 letters, digits or underscores")
	}
	if reservedUsernames[strings.ToLower(u)] {
		return "", badRequest("that username is reserved")
	}
	var other int64
	err := s.db.QueryRow(`SELECT id FROM users WHERE username = ? COLLATE NOCASE AND id != ?`, u, userID).Scan(&other)
	switch {
	case err == nil:
		return "", conflict("username already taken")
	case errors.Is(err, sql.ErrNoRows):
		return u, nil
	default:
		return "", err
	}
}

func validDisplayName(raw string) (string, error) {
	d := strings.TrimSpace(raw)
	if n := utf8.RuneCountInString(d); n == 0 || n > 50 {
		return "", badRequest("display name must be 1-50 characters")
	}
	return d, nil
}

// shortText trims a free-text field and caps its length in characters.
func shortText(raw string, max int, name string) (string, error) {
	t := strings.TrimSpace(raw)
	if utf8.RuneCountInString(t) > max {
		return "", badRequest(fmt.Sprintf("%s must be at most %d characters", name, max))
	}
	return t, nil
}

// validAvatar accepts "" (no picture) or an uploaded picture under /media.
func (s *server) validAvatar(raw string) (string, error) {
	a := strings.TrimSpace(raw)
	if a == "" {
		return "", nil
	}
	name, ok := uploadName(a)
	if !ok || !avatarExts[strings.ToLower(filepath.Ext(name))] {
		return "", badRequest("profile pictures must be pictures uploaded via /api/media")
	}
	if !s.uploadExists(name) {
		return "", badRequest("unknown media file " + name)
	}
	return a, nil
}

func (s *server) currentAvatar(userID int64) (string, error) {
	var url string
	if err := s.db.QueryRow(`SELECT avatar_url FROM users WHERE id = ?`, userID).Scan(&url); err != nil || url == "" {
		return "", badRequest("no profile picture to position")
	}
	return url, nil
}

// cropFor is the crop to store: the requested one on the picture's real
// aspect ratio, or the centred square when none was requested.
func (s *server) cropFor(url string, requested *AvatarCrop) (AvatarCrop, error) {
	aspect := s.pictureAspect(url)
	if requested == nil {
		return defaultCrop(aspect), nil
	}
	crop := *requested
	if aspect > 0 {
		crop.Aspect = aspect
	}
	if !crop.valid() {
		return crop, badRequest("avatar_crop must be a square inside the picture: x, y, w in 0-1")
	}
	return crop.rounded(), nil
}
