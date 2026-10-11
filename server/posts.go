package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"
)

type Post struct {
	ID           int64          `json:"id"`
	Author       User           `json:"author"`
	GroupID      *int64         `json:"group_id"`
	GroupName    string         `json:"group_name,omitempty"`
	Body         string         `json:"body"`
	CreatedAt    string         `json:"created_at"`
	CommentCount int            `json:"comment_count"`
	Reactions    map[string]int `json:"reactions"`   // emoji -> count
	MyReaction   string         `json:"my_reaction"` // "" if none
}

type Comment struct {
	ID        int64  `json:"id"`
	PostID    int64  `json:"post_id"`
	Author    User   `json:"author"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
}

// postSelect expects the viewer's user id as its first bound parameter.
const postSelect = `
SELECT p.id, p.body, p.created_at, p.group_id, COALESCE(g.name, ''),
       ` + userCols + `,
       (SELECT COUNT(*) FROM comments c WHERE c.post_id = p.id),
       COALESCE((SELECT json_group_object(emoji, n) FROM
            (SELECT emoji, COUNT(*) AS n FROM reactions r WHERE r.post_id = p.id GROUP BY emoji)), '{}'),
       COALESCE((SELECT emoji FROM reactions r WHERE r.post_id = p.id AND r.user_id = ?), '')
FROM posts p
JOIN users u ON u.id = p.author_id
LEFT JOIN groups g ON g.id = p.group_id
`

func scanPost(sc interface{ Scan(...any) error }) (Post, error) {
	var p Post
	var reactions string
	err := sc.Scan(&p.ID, &p.Body, &p.CreatedAt, &p.GroupID, &p.GroupName,
		&p.Author.ID, &p.Author.Username, &p.Author.DisplayName, &p.Author.CreatedAt,
		&p.CommentCount, &reactions, &p.MyReaction)
	if err != nil {
		return p, err
	}
	p.Reactions = map[string]int{}
	_ = json.Unmarshal([]byte(reactions), &p.Reactions)
	return p, nil
}

// queryPosts runs postSelect + tail for viewer, binding args after the viewer id.
func (s *server) queryPosts(viewer int64, tail string, args ...any) ([]Post, error) {
	rows, err := s.db.Query(postSelect+tail, append([]any{viewer}, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	posts := []Post{}
	for rows.Next() {
		p, err := scanPost(rows)
		if err != nil {
			return nil, err
		}
		posts = append(posts, p)
	}
	return posts, rows.Err()
}

func (s *server) getPost(viewer, id int64) (Post, error) {
	return scanPost(s.db.QueryRow(postSelect+`WHERE p.id = ?`, viewer, id))
}

// feed: my own posts, friends' profile posts, and posts in groups I belong to.
func (s *server) feed(w http.ResponseWriter, r *http.Request, me *User) {
	before, limit := pagination(r)
	posts, err := s.queryPosts(me.ID, `
		WHERE p.id < ?2 AND (
			(p.group_id IS NULL AND (p.author_id = ?1 OR p.author_id IN (
				SELECT CASE WHEN from_id = ?1 THEN to_id ELSE from_id END FROM friendships
				WHERE status = 'accepted' AND (from_id = ?1 OR to_id = ?1))))
			OR p.group_id IN (SELECT group_id FROM group_members WHERE user_id = ?1))
		ORDER BY p.id DESC LIMIT ?3`, before, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, posts)
}

func (s *server) createPost(w http.ResponseWriter, r *http.Request, me *User) {
	var in struct {
		Body    string `json:"body"`
		GroupID *int64 `json:"group_id"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	in.Body = strings.TrimSpace(in.Body)
	if !clampLen(in.Body, 5000) {
		writeError(w, http.StatusBadRequest, "body must be 1-5000 characters")
		return
	}
	if in.GroupID != nil {
		role, err := s.groupRole(*in.GroupID, me.ID)
		if err != nil || role == "" {
			writeError(w, http.StatusForbidden, "join the group to post in it")
			return
		}
	}
	res, err := s.db.Exec(`INSERT INTO posts(author_id, group_id, body, created_at) VALUES (?, ?, ?, ?)`,
		me.ID, in.GroupID, in.Body, nowString())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	id, _ := res.LastInsertId()
	s.respondPost(w, http.StatusCreated, me.ID, id)
}

func (s *server) respondPost(w http.ResponseWriter, code int, viewer, id int64) {
	p, err := s.getPost(viewer, id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "post not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, code, p)
}

func (s *server) getPostHandler(w http.ResponseWriter, r *http.Request, me *User) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	s.respondPost(w, http.StatusOK, me.ID, id)
}

// deletePost: allowed for the author, or the admin of the group it was posted in.
func (s *server) deletePost(w http.ResponseWriter, r *http.Request, me *User) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var author int64
	var groupID *int64
	err := s.db.QueryRow(`SELECT author_id, group_id FROM posts WHERE id = ?`, id).Scan(&author, &groupID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "post not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	allowed := author == me.ID
	if !allowed && groupID != nil {
		role, _ := s.groupRole(*groupID, me.ID)
		allowed = role == "admin"
	}
	if !allowed {
		writeError(w, http.StatusForbidden, "not allowed")
		return
	}
	if _, err := s.db.Exec(`DELETE FROM posts WHERE id = ?`, id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) listComments(w http.ResponseWriter, r *http.Request, me *User) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	after := queryInt(r, "after", 0)
	rows, err := s.db.Query(`SELECT c.id, c.post_id, c.body, c.created_at, `+userCols+`
		FROM comments c JOIN users u ON u.id = c.author_id
		WHERE c.post_id = ? AND c.id > ? ORDER BY c.id ASC LIMIT 200`, id, after)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []Comment{}
	for rows.Next() {
		var c Comment
		if err := rows.Scan(&c.ID, &c.PostID, &c.Body, &c.CreatedAt,
			&c.Author.ID, &c.Author.Username, &c.Author.DisplayName, &c.Author.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, c)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) createComment(w http.ResponseWriter, r *http.Request, me *User) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Body string `json:"body"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	in.Body = strings.TrimSpace(in.Body)
	if !clampLen(in.Body, 2000) {
		writeError(w, http.StatusBadRequest, "body must be 1-2000 characters")
		return
	}
	var exists int
	if err := s.db.QueryRow(`SELECT 1 FROM posts WHERE id = ?`, id).Scan(&exists); err != nil {
		writeError(w, http.StatusNotFound, "post not found")
		return
	}
	createdAt := nowString()
	res, err := s.db.Exec(`INSERT INTO comments(post_id, author_id, body, created_at) VALUES (?, ?, ?, ?)`,
		id, me.ID, in.Body, createdAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	cid, _ := res.LastInsertId()
	writeJSON(w, http.StatusCreated, Comment{ID: cid, PostID: id, Author: *me, Body: in.Body, CreatedAt: createdAt})
}

// setReaction upserts the caller's single reaction on a post. Any emoji works.
func (s *server) setReaction(w http.ResponseWriter, r *http.Request, me *User) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Emoji string `json:"emoji"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	in.Emoji = strings.TrimSpace(in.Emoji)
	if in.Emoji == "" || len(in.Emoji) > 32 || !utf8.ValidString(in.Emoji) || utf8.RuneCountInString(in.Emoji) > 8 {
		writeError(w, http.StatusBadRequest, "invalid emoji")
		return
	}
	var exists int
	if err := s.db.QueryRow(`SELECT 1 FROM posts WHERE id = ?`, id).Scan(&exists); err != nil {
		writeError(w, http.StatusNotFound, "post not found")
		return
	}
	_, err := s.db.Exec(`INSERT INTO reactions(post_id, user_id, emoji) VALUES (?, ?, ?)
		ON CONFLICT(post_id, user_id) DO UPDATE SET emoji = excluded.emoji`, id, me.ID, in.Emoji)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.respondPost(w, http.StatusOK, me.ID, id)
}

func (s *server) deleteReaction(w http.ResponseWriter, r *http.Request, me *User) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if _, err := s.db.Exec(`DELETE FROM reactions WHERE post_id = ? AND user_id = ?`, id, me.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.respondPost(w, http.StatusOK, me.ID, id)
}
