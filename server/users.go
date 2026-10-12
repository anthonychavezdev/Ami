package main

import (
	"net/http"
	"strings"
)

// User is what everyone sees of an account. Me adds the private settings,
// UserProfile what a profile page shows.
type User struct {
	ID          int64      `json:"id"`
	Username    string     `json:"username"`
	DisplayName string     `json:"display_name"`
	CreatedAt   string     `json:"created_at"`
	AvatarURL   string     `json:"avatar_url,omitempty"` // an uploaded picture under /media, or empty for initials
	AvatarCrop  AvatarCrop `json:"avatar_crop"`          // the part of the picture shown in the circle (W == 0: whole picture, centred)
	Pronouns    string     `json:"pronouns,omitempty"`
	Online      bool       `json:"online"` // has a live connection; filled in by markPresence (presence.go) on output
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

type UserProfile struct {
	User
	Bio          string `json:"bio"`
	FriendStatus string `json:"friend_status"` // self | none | friends | outgoing | incoming
	FriendCount  int    `json:"friend_count"`
	PostCount    int    `json:"post_count"`
}

// searchUsers finds people by username or display name, with how each one
// relates to the caller.
func (s *server) searchUsers(w http.ResponseWriter, r *http.Request, me *User) error {
	like := "%" + escapeLike(strings.TrimSpace(r.URL.Query().Get("q"))) + "%"
	users, err := queryAll(s.db, scanUser, `SELECT `+userCols+` FROM users u
		WHERE u.id != ? AND (u.username LIKE ? ESCAPE '\' OR u.display_name LIKE ? ESCAPE '\')
		ORDER BY u.username LIMIT 25`, me.ID, like, like)
	if err != nil {
		return err
	}
	return ok(w, mapSlice(users, func(u User) UserProfile {
		status, _ := s.friendStatus(me.ID, u.ID)
		return UserProfile{User: u, FriendStatus: status}
	}))
}

// escapeLike makes a search term literal inside a LIKE pattern.
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

// profileOf is a user as seen by viewer: bio, friendship and counts included.
func (s *server) profileOf(viewer int64, u User) UserProfile {
	p := UserProfile{User: u}
	_ = s.db.QueryRow(`SELECT bio FROM users WHERE id = ?`, u.ID).Scan(&p.Bio)
	p.FriendStatus, _ = s.friendStatus(viewer, u.ID)
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM friendships WHERE status = 'accepted' AND (from_id = ?1 OR to_id = ?1)`, u.ID).Scan(&p.FriendCount)
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM posts WHERE author_id = ? AND group_id IS NULL`, u.ID).Scan(&p.PostCount)
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
