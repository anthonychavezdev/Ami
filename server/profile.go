package main

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"
)

// The caller's own profile and settings: PUT /api/me and PUT /api/me/presence.

// Names nobody may take: "deleted" is the placeholder author of removed comments.
var reservedUsernames = map[string]bool{"deleted": true, "admin": true, "ami": true, "me": true, "system": true}

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

// ---- appearing offline

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
	return ok(w, in)
}
