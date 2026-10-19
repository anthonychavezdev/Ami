package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"
)

// The caller's own profile and settings: PUT /api/me and PUT /api/me/presence.

// Names nobody may take: "deleted" is the placeholder author of removed comments.
var reservedUsernames = map[string]bool{"deleted": true, "admin": true, "ami": true, "me": true, "system": true}

// profileInput is a PUT /api/me body: only the fields sent are changed. The
// bio is Markdown, which the app renders and the server keeps as text.
type profileInput struct {
	Username    *string   `json:"username"`
	DisplayName *string   `json:"display_name"`
	Bio         *string   `json:"bio"`
	Pronouns    *string   `json:"pronouns"`
	Interests   *[]string `json:"interests"`
	Links       *[]Link   `json:"links"`
	AvatarURL   *string   `json:"avatar_url"`
	AvatarCrop  *Crop     `json:"avatar_crop"`
	CoverURL    *string   `json:"cover_url"`
	CoverCrop   *Crop     `json:"cover_crop"`
}

// Link is one of a person's other places: a label and an http(s) URL.
type Link struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

const (
	maxInterests    = 20
	maxInterestLen  = 30
	maxLinks        = 10
	maxLinkLabelLen = 30
)

func decodeInterests(raw string) []string {
	out := []string{}
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}

func decodeLinks(raw string) []Link {
	out := []Link{}
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}

// cleanInterests trims the tags, drops empty ones and repeats (ignoring
// case), and caps their count and length.
func cleanInterests(raw []string) ([]string, error) {
	out := []string{}
	seen := map[string]bool{}
	for _, t := range raw {
		t = strings.TrimSpace(t)
		if t == "" || seen[strings.ToLower(t)] {
			continue
		}
		if utf8.RuneCountInString(t) > maxInterestLen {
			return nil, badRequest(fmt.Sprintf("an interest must be at most %d characters", maxInterestLen))
		}
		seen[strings.ToLower(t)] = true
		out = append(out, t)
	}
	if len(out) > maxInterests {
		return nil, badRequest(fmt.Sprintf("at most %d interests", maxInterests))
	}
	return out, nil
}

// cleanLinks checks each link's URL (http or https, like a post's link) and
// label; a missing label is the site's host.
func cleanLinks(raw []Link) ([]Link, error) {
	if len(raw) > maxLinks {
		return nil, badRequest(fmt.Sprintf("at most %d links", maxLinks))
	}
	out := []Link{}
	for _, l := range raw {
		u, err := validLink(l.URL)
		if err != nil {
			return nil, badRequest(err.Error())
		}
		label := strings.TrimSpace(l.Label)
		if utf8.RuneCountInString(label) > maxLinkLabelLen {
			return nil, badRequest(fmt.Sprintf("a link label must be at most %d characters", maxLinkLabelLen))
		}
		if label == "" {
			label = strings.TrimPrefix(u.Host, "www.")
		}
		out = append(out, Link{Label: label, URL: l.URL})
	}
	return out, nil
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
func (s *server) profilePatch(userID string, in profileInput) (columnPatch, error) {
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
	if in.Interests != nil {
		tags, err := cleanInterests(*in.Interests)
		if err != nil {
			return p, err
		}
		p = p.with("interests", jsonString(tags))
	}
	if in.Links != nil {
		links, err := cleanLinks(*in.Links)
		if err != nil {
			return p, err
		}
		p = p.with("links", jsonString(links))
	}
	p, err := s.picturePatch(p, userID, avatarPicture, in.AvatarURL, in.AvatarCrop)
	if err != nil {
		return p, err
	}
	return s.picturePatch(p, userID, coverPicture, in.CoverURL, in.CoverCrop)
}

// newUsername checks a username change: valid, not reserved, nobody else's.
func (s *server) newUsername(raw string, userID string) (string, error) {
	u := strings.TrimSpace(raw)
	if !usernameRe.MatchString(u) {
		return "", badRequest("username must be 3-24 letters, digits or underscores")
	}
	if reservedUsernames[strings.ToLower(u)] {
		return "", badRequest("that username is reserved")
	}
	var other string
	err := s.db.QueryRow(`SELECT id FROM users WHERE username = ? AND id != ?`, u, userID).Scan(&other) // the column's collation ignores case
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

func (s *server) invisible(userID string) bool {
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
