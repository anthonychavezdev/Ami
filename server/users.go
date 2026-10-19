package main

import (
	"net/http"
	"strings"
)

// User is what everyone sees of an account. Me adds the private settings,
// UserProfile what a profile page shows.
type User struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	CreatedAt   string `json:"created_at"`
	AvatarURL   string `json:"avatar_url,omitempty"` // an uploaded picture under /media, or empty for initials
	AvatarCrop  Crop   `json:"avatar_crop"`          // the part of the picture shown in the circle (W == 0: whole picture, centred)
	Pronouns    string `json:"pronouns,omitempty"`
	Online      bool   `json:"online"` // has a live connection; filled in by markPresence (presence.go) on output
}

// userCols selects a User from the users table (alias u); fields() scans it.
// A new column goes in both, in the same position, and in the schema (db.go).
const userCols = `u.id, u.username, u.display_name, u.created_at, u.avatar_url, u.pronouns, u.avatar_crop`

// fields are the scan targets for userCols, in that order.
func (u *User) fields() []any {
	return []any{&u.ID, &u.Username, &u.DisplayName, &u.CreatedAt, &u.AvatarURL, &u.Pronouns, &u.AvatarCrop}
}

func scanUser(sc scanner) (User, error) {
	var u User
	return u, sc.Scan(u.fields()...)
}

func (s *server) userByID(id string) (User, error) {
	return scanUser(s.db.QueryRow(`SELECT `+userCols+` FROM users u WHERE u.id = ?`, id))
}

// Me is the caller's own user with private settings and the full profile.
type Me struct {
	User
	Invisible          bool     `json:"invisible"` // appear offline to everyone
	Bio                string   `json:"bio"`       // Markdown
	Interests          []string `json:"interests"` // tags, shown on the profile and searched
	Links              []Link   `json:"links"`     // other places of theirs
	CoverURL           string   `json:"cover_url"` // the wide picture across the top of the profile
	CoverCrop          Crop     `json:"cover_crop"`
	MustChangePassword bool     `json:"must_change_password"` // signed in with a temporary password: set a new one first
}

func (s *server) meFor(u User) Me {
	m := Me{User: u}
	var interests, links string
	_ = s.db.QueryRow(`SELECT invisible, bio, interests, links, cover_url, cover_crop, must_change_password FROM users WHERE id = ?`, u.ID).
		Scan(&m.Invisible, &m.Bio, &interests, &links, &m.CoverURL, &m.CoverCrop, &m.MustChangePassword)
	m.Interests, m.Links = decodeInterests(interests), decodeLinks(links)
	return m
}

// UserProfile is a user as a profile page shows them. Search results carry
// only the interests (what matched) and the friendship status.
type UserProfile struct {
	User
	Bio          string   `json:"bio"`
	Interests    []string `json:"interests"`
	Links        []Link   `json:"links"`
	CoverURL     string   `json:"cover_url,omitempty"`
	CoverCrop    Crop     `json:"cover_crop"`
	FriendStatus string   `json:"friend_status"` // self | none | friends | outgoing | incoming
	FriendCount  int      `json:"friend_count"`
	PostCount    int      `json:"post_count"`
}

// searchUsers finds people by username, display name or interest, with how
// each one relates to the caller and the interests that show what matched.
func (s *server) searchUsers(w http.ResponseWriter, r *http.Request, me *User) error {
	like := "%" + escapeLike(strings.TrimSpace(r.URL.Query().Get("q"))) + "%"
	type hit struct {
		user      User
		interests string
	}
	type hitWithStatus struct {
		hit
		status string
	}
	hits, err := queryAll(s.db, func(sc scanner) (hitWithStatus, error) {
		var h hitWithStatus
		return h, sc.Scan(fields(h.user.fields(), []any{&h.interests, &h.status})...)
	}, `SELECT `+userCols+`, u.interests,
			COALESCE((SELECT CASE WHEN f.status = 'accepted' THEN 'friends' WHEN f.from_id = ? THEN 'outgoing' ELSE 'incoming' END
				FROM friendships f WHERE (f.from_id = ? AND f.to_id = u.id) OR (f.from_id = u.id AND f.to_id = ?)), 'none')
		FROM users u
		WHERE u.id != ? AND (u.username LIKE ? OR u.display_name LIKE ?
			OR JSON_SEARCH(LOWER(u.interests), 'one', ?) IS NOT NULL)
		ORDER BY u.username LIMIT 25`, me.ID, me.ID, me.ID, me.ID, like, like, strings.ToLower(like))
	if err != nil {
		return err
	}
	return ok(w, mapSlice(hits, func(h hitWithStatus) UserProfile {
		return UserProfile{User: h.user, Interests: decodeInterests(h.interests), Links: []Link{}, FriendStatus: h.status}
	}))
}

// escapeLike makes a search term literal inside a LIKE pattern (the escape
// character is the default backslash; JSON_SEARCH uses the same patterns).
func escapeLike(q string) string { return strings.NewReplacer("%", "\\%", "_", "\\_").Replace(q) }

func (s *server) getUser(w http.ResponseWriter, r *http.Request, me *User) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	u, err := s.userByID(id)
	if err != nil {
		return orNotFound(err, "user not found")
	}
	return ok(w, s.profileOf(me.ID, u))
}

// profileOf is a user as seen by viewer: bio, cover, interests, links,
// friendship and counts included.
func (s *server) profileOf(viewer string, u User) UserProfile {
	p := UserProfile{User: u}
	var interests, links string
	_ = s.db.QueryRow(`SELECT bio, interests, links, cover_url, cover_crop,
			(SELECT COUNT(*) FROM friendships WHERE status = 'accepted' AND (from_id = u.id OR to_id = u.id)),
			(SELECT COUNT(*) FROM posts WHERE author_id = u.id AND group_id IS NULL)
		FROM users u WHERE u.id = ?`, u.ID).Scan(&p.Bio, &interests, &links, &p.CoverURL, &p.CoverCrop, &p.FriendCount, &p.PostCount)
	p.Interests, p.Links = decodeInterests(interests), decodeLinks(links)
	p.FriendStatus, _ = s.friendStatus(viewer, u.ID)
	return p
}

func (s *server) userPosts(w http.ResponseWriter, r *http.Request, me *User) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	before, limit := pagination(r)
	posts, err := s.queryPosts(me.ID, `WHERE p.author_id = ? AND p.group_id IS NULL AND p.id < ? ORDER BY p.id DESC LIMIT ?`, id, before, limit)
	if err != nil {
		return err
	}
	return ok(w, posts)
}
