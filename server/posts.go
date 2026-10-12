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
	Attachments  []Attachment   `json:"attachments"`
	CreatedAt    string         `json:"created_at"`
	EditedAt     *string        `json:"edited_at"` // null until the author edits it
	CommentCount int            `json:"comment_count"`
	Reactions    map[string]int `json:"reactions"`   // emoji -> count
	MyReaction   string         `json:"my_reaction"` // "" if none
}

type Comment struct {
	ID        int64   `json:"id"`
	PostID    int64   `json:"post_id"`
	ParentID  *int64  `json:"parent_id"` // reply to another comment of the same post
	Author    User    `json:"author"`
	Body      string  `json:"body"`
	CreatedAt string  `json:"created_at"`
	EditedAt  *string `json:"edited_at"`
	Deleted   bool    `json:"deleted"` // body and author read [deleted]; the row stays so replies keep their place
}

// deletedUser stands in for the author of a deleted comment.
var deletedUser = User{Username: "deleted", DisplayName: "[deleted]"}

const commentCols = `c.id, c.post_id, c.parent_id, c.body, c.created_at, c.edited_at, c.deleted_at, ` + userCols

func scanComment(sc interface{ Scan(...any) error }) (Comment, error) {
	var c Comment
	var deleted *string
	err := sc.Scan(&c.ID, &c.PostID, &c.ParentID, &c.Body, &c.CreatedAt, &c.EditedAt, &deleted,
		&c.Author.ID, &c.Author.Username, &c.Author.DisplayName, &c.Author.CreatedAt, &c.Author.AvatarURL, &c.Author.Pronouns, &c.Author.AvatarCrop)
	if deleted != nil {
		c.Deleted, c.Body, c.EditedAt, c.Author = true, "[deleted]", nil, deletedUser
	}
	return c, err
}

func (s *server) loadComment(id int64) (Comment, error) {
	return scanComment(s.db.QueryRow(`SELECT `+commentCols+` FROM comments c JOIN users u ON u.id = c.author_id WHERE c.id = ?`, id))
}

// commentAudience is everyone who can see the comment's post.
func (s *server) commentAudience(postID int64) []int64 {
	var author int64
	var groupID *int64
	if err := s.db.QueryRow(`SELECT author_id, group_id FROM posts WHERE id = ?`, postID).Scan(&author, &groupID); err != nil {
		return nil
	}
	return s.postAudience(author, groupID)
}

// postSelect expects the viewer's user id as its first bound parameter.
const postSelect = `
SELECT p.id, p.body, p.attachments, p.created_at, p.edited_at, p.group_id, COALESCE(g.name, ''),
       ` + userCols + `,
       (SELECT COUNT(*) FROM comments c WHERE c.post_id = p.id AND c.deleted_at IS NULL),
       COALESCE((SELECT json_group_object(emoji, n) FROM
            (SELECT emoji, COUNT(*) AS n FROM reactions r WHERE r.post_id = p.id GROUP BY emoji)), '{}'),
       COALESCE((SELECT emoji FROM reactions r WHERE r.post_id = p.id AND r.user_id = ?), '')
FROM posts p
JOIN users u ON u.id = p.author_id
LEFT JOIN groups g ON g.id = p.group_id
`

func scanPost(sc interface{ Scan(...any) error }) (Post, error) {
	var p Post
	var reactions, attachments string
	err := sc.Scan(&p.ID, &p.Body, &attachments, &p.CreatedAt, &p.EditedAt, &p.GroupID, &p.GroupName,
		&p.Author.ID, &p.Author.Username, &p.Author.DisplayName, &p.Author.CreatedAt, &p.Author.AvatarURL, &p.Author.Pronouns, &p.Author.AvatarCrop,
		&p.CommentCount, &reactions, &p.MyReaction)
	if err != nil {
		return p, err
	}
	p.Reactions = map[string]int{}
	_ = json.Unmarshal([]byte(reactions), &p.Reactions)
	p.Attachments = []Attachment{}
	_ = json.Unmarshal([]byte(attachments), &p.Attachments)
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

// postAudience lists who sees a post in their feed: the author and their
// friends for profile posts, every member for group posts.
func (s *server) postAudience(authorID int64, groupID *int64) []int64 {
	var rows *sql.Rows
	var err error
	if groupID != nil {
		rows, err = s.db.Query(`SELECT user_id FROM group_members WHERE group_id = ?`, *groupID)
	} else {
		rows, err = s.db.Query(`SELECT CASE WHEN from_id = ?1 THEN to_id ELSE from_id END FROM friendships
			WHERE status = 'accepted' AND (from_id = ?1 OR to_id = ?1)`, authorID)
	}
	ids := []int64{authorID}
	if err != nil {
		return ids
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil && id != authorID {
			ids = append(ids, id)
		}
	}
	return ids
}

// broadcastPost pushes a post's current state to everyone who sees it, each
// with their own reaction filled in, so feeds update without a refresh.
func (s *server) broadcastPost(typ string, id int64) {
	base, err := s.getPost(0, id)
	if err != nil {
		return
	}
	for _, uid := range s.postAudience(base.Author.ID, base.GroupID) {
		if !s.hub.online(uid) {
			continue
		}
		p, err := s.getPost(uid, id)
		if err != nil {
			continue
		}
		s.hub.sendTo(uid, map[string]any{"type": typ, "post": p})
	}
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
		Body        string       `json:"body"`
		GroupID     *int64       `json:"group_id"`
		Attachments []Attachment `json:"attachments"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	in.Body = strings.TrimSpace(in.Body)
	if in.Body == "" && len(in.Attachments) == 0 {
		writeError(w, http.StatusBadRequest, "a post needs text or an attachment")
		return
	}
	if len(in.Body) > 5000 {
		writeError(w, http.StatusBadRequest, "body must be at most 5000 characters")
		return
	}
	if in.Attachments == nil {
		in.Attachments = []Attachment{}
	}
	if err := s.validateAttachments(in.Attachments); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	attachments, _ := json.Marshal(in.Attachments)
	if in.GroupID != nil {
		role, err := s.groupRole(*in.GroupID, me.ID)
		if err != nil || role == "" {
			writeError(w, http.StatusForbidden, "join the group to post in it")
			return
		}
	}
	res, err := s.db.Exec(`INSERT INTO posts(author_id, group_id, body, attachments, created_at) VALUES (?, ?, ?, ?, ?)`,
		me.ID, in.GroupID, in.Body, string(attachments), nowString())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	id, _ := res.LastInsertId()
	s.broadcastPost("post.created", id)
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
	audience := s.postAudience(author, groupID)
	if _, err := s.db.Exec(`DELETE FROM posts WHERE id = ?`, id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.hub.sendToMany(audience, map[string]any{"type": "post.deleted", "id": id, "group_id": groupID})
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) listComments(w http.ResponseWriter, r *http.Request, me *User) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	after := queryInt(r, "after", 0)
	rows, err := s.db.Query(`SELECT `+commentCols+`
		FROM comments c JOIN users u ON u.id = c.author_id
		WHERE c.post_id = ? AND c.id > ? ORDER BY c.id ASC LIMIT 500`, id, after)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []Comment{}
	for rows.Next() {
		c, err := scanComment(rows)
		if err != nil {
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
		Body     string `json:"body"`
		ParentID *int64 `json:"parent_id"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	in.Body = strings.TrimSpace(in.Body)
	if !clampLen(in.Body, 2000) {
		writeError(w, http.StatusBadRequest, "body must be 1-2000 characters")
		return
	}
	var author int64
	var groupID *int64
	if err := s.db.QueryRow(`SELECT author_id, group_id FROM posts WHERE id = ?`, id).Scan(&author, &groupID); err != nil {
		writeError(w, http.StatusNotFound, "post not found")
		return
	}
	if in.ParentID != nil {
		parent, err := s.loadComment(*in.ParentID)
		if err != nil || parent.PostID != id || parent.Deleted {
			writeError(w, http.StatusBadRequest, "cannot reply to that comment")
			return
		}
	}
	createdAt := nowString()
	res, err := s.db.Exec(`INSERT INTO comments(post_id, parent_id, author_id, body, created_at) VALUES (?, ?, ?, ?, ?)`,
		id, in.ParentID, me.ID, in.Body, createdAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	cid, _ := res.LastInsertId()
	c := Comment{ID: cid, PostID: id, ParentID: in.ParentID, Author: *me, Body: in.Body, CreatedAt: createdAt}
	s.hub.sendToMany(s.postAudience(author, groupID), map[string]any{"type": "comment.created", "comment": c})
	s.broadcastPost("post.updated", id)
	writeJSON(w, http.StatusCreated, c)
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
	s.broadcastPost("post.updated", id)
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
	s.broadcastPost("post.updated", id)
	s.respondPost(w, http.StatusOK, me.ID, id)
}

// updatePost lets the author change a post's text and attachments; everyone
// who can see it gets a post.updated event.
func (s *server) updatePost(w http.ResponseWriter, r *http.Request, me *User) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Body        string       `json:"body"`
		Attachments []Attachment `json:"attachments"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	in.Body = strings.TrimSpace(in.Body)
	if in.Body == "" && len(in.Attachments) == 0 {
		writeError(w, http.StatusBadRequest, "a post needs text or an attachment")
		return
	}
	if len(in.Body) > 5000 {
		writeError(w, http.StatusBadRequest, "body must be at most 5000 characters")
		return
	}
	if in.Attachments == nil {
		in.Attachments = []Attachment{}
	}
	if err := s.validateAttachments(in.Attachments); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var author int64
	err := s.db.QueryRow(`SELECT author_id FROM posts WHERE id = ?`, id).Scan(&author)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "post not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if author != me.ID {
		writeError(w, http.StatusForbidden, "only the author can edit a post")
		return
	}
	attachments, _ := json.Marshal(in.Attachments)
	if _, err := s.db.Exec(`UPDATE posts SET body = ?, attachments = ?, edited_at = ? WHERE id = ?`, in.Body, string(attachments), nowString(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	p, err := s.getPost(me.ID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.broadcastPost("post.updated", id)
	writeJSON(w, http.StatusOK, p)
}

// updateComment lets the author change a comment's text.
func (s *server) updateComment(w http.ResponseWriter, r *http.Request, me *User) {
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
	c, err := s.loadComment(id)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && c.Deleted) {
		writeError(w, http.StatusNotFound, "comment not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if c.Author.ID != me.ID {
		writeError(w, http.StatusForbidden, "only the author can edit a comment")
		return
	}
	if _, err := s.db.Exec(`UPDATE comments SET body = ?, edited_at = ? WHERE id = ?`, in.Body, nowString(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if c, err = s.loadComment(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.hub.sendToMany(s.commentAudience(c.PostID), map[string]any{"type": "comment.updated", "comment": c})
	writeJSON(w, http.StatusOK, c)
}

// deleteComment removes the author's comment. One that has replies is kept as
// a [deleted] placeholder so the thread below it stays readable; placeholders
// whose last reply disappears are removed too.
func (s *server) deleteComment(w http.ResponseWriter, r *http.Request, me *User) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	c, err := s.loadComment(id)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && c.Deleted) {
		writeError(w, http.StatusNotFound, "comment not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if c.Author.ID != me.ID {
		writeError(w, http.StatusForbidden, "only the author can delete a comment")
		return
	}
	var replies int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM comments WHERE parent_id = ?`, id).Scan(&replies); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if replies > 0 {
		_, err = s.db.Exec(`UPDATE comments SET deleted_at = ?, body = '' WHERE id = ?`, nowString(), id)
	} else {
		_, err = s.db.Exec(`DELETE FROM comments WHERE id = ?`, id)
		// Walk up: a [deleted] parent with nothing left under it goes too.
		for parent := c.ParentID; parent != nil && err == nil; {
			var deleted *string
			var next *int64
			var n int
			if s.db.QueryRow(`SELECT deleted_at, parent_id, (SELECT COUNT(*) FROM comments x WHERE x.parent_id = c.id) FROM comments c WHERE id = ?`, *parent).Scan(&deleted, &next, &n) != nil || deleted == nil || n > 0 {
				break
			}
			_, err = s.db.Exec(`DELETE FROM comments WHERE id = ?`, *parent)
			parent = next
		}
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.hub.sendToMany(s.commentAudience(c.PostID), map[string]any{"type": "comment.deleted", "post_id": c.PostID, "id": id})
	s.broadcastPost("post.updated", c.PostID)
	w.WriteHeader(http.StatusNoContent)
}
