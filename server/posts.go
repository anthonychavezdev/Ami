package main

import (
	"encoding/json"
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

// ---- reading posts

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

func scanPost(sc scanner) (Post, error) {
	var p Post
	var reactions, attachments string
	err := sc.Scan(fields([]any{&p.ID, &p.Body, &attachments, &p.CreatedAt, &p.EditedAt, &p.GroupID, &p.GroupName},
		p.Author.fields(), []any{&p.CommentCount, &reactions, &p.MyReaction})...)
	if err != nil {
		return p, err
	}
	p.Reactions = map[string]int{}
	_ = json.Unmarshal([]byte(reactions), &p.Reactions)
	p.Attachments = decodeAttachments(attachments)
	return p, nil
}

// decodeAttachments turns a stored JSON column into a (never nil) slice.
func decodeAttachments(raw string) []Attachment {
	out := []Attachment{}
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}

// queryPosts runs postSelect + tail for viewer, binding args after the viewer id.
func (s *server) queryPosts(viewer int64, tail string, args ...any) ([]Post, error) {
	return queryAll(s.db, scanPost, postSelect+tail, append([]any{viewer}, args...)...)
}

func (s *server) getPost(viewer, id int64) (Post, error) {
	return scanPost(s.db.QueryRow(postSelect+`WHERE p.id = ?`, viewer, id))
}

func (s *server) respondPost(w http.ResponseWriter, code int, viewer, id int64) error {
	p, err := s.getPost(viewer, id)
	if err != nil {
		return orNotFound(err, "post not found")
	}
	writeJSON(w, code, p)
	return nil
}

// postOwner is who wrote a post and which group it was posted in.
type postOwner struct {
	author  int64
	groupID *int64
}

func (s *server) postOwner(id int64) (postOwner, error) {
	var o postOwner
	err := s.db.QueryRow(`SELECT author_id, group_id FROM posts WHERE id = ?`, id).Scan(&o.author, &o.groupID)
	return o, orNotFound(err, "post not found")
}

// postAudience lists who sees a post in their feed: the author and their
// friends for profile posts, every member for group posts.
func (s *server) postAudience(authorID int64, groupID *int64) []int64 {
	if groupID != nil {
		return append([]int64{authorID}, without(s.groupMemberIDs(*groupID), authorID)...)
	}
	return append([]int64{authorID}, s.friendIDs(authorID)...)
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
		if p, err := s.getPost(uid, id); err == nil {
			s.hub.sendTo(uid, postEvent{Type: typ, Post: p})
		}
	}
}

// feed: my own posts, friends' profile posts, and posts in groups I belong to.
func (s *server) feed(w http.ResponseWriter, r *http.Request, me *User) error {
	before, limit := pagination(r)
	posts, err := s.queryPosts(me.ID, `
		WHERE p.id < ?2 AND (
			(p.group_id IS NULL AND (p.author_id = ?1 OR p.author_id IN (
				SELECT CASE WHEN from_id = ?1 THEN to_id ELSE from_id END FROM friendships
				WHERE status = 'accepted' AND (from_id = ?1 OR to_id = ?1))))
			OR p.group_id IN (SELECT group_id FROM group_members WHERE user_id = ?1))
		ORDER BY p.id DESC LIMIT ?3`, before, limit)
	if err != nil {
		return err
	}
	return ok(w, posts)
}

func (s *server) getPostHandler(w http.ResponseWriter, r *http.Request, me *User) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	return s.respondPost(w, http.StatusOK, me.ID, id)
}

// ---- writing posts

// postInput is the body of POST /api/posts and PUT /api/posts/{id}.
type postInput struct {
	Body        string       `json:"body"`
	GroupID     *int64       `json:"group_id"`
	Attachments []Attachment `json:"attachments"`
}

// cleanPostInput trims the text and checks it and the attachments.
func (s *server) cleanPostInput(in postInput) (postInput, error) {
	in.Body = strings.TrimSpace(in.Body)
	if in.Body == "" && len(in.Attachments) == 0 {
		return in, badRequest("a post needs text or an attachment")
	}
	if len(in.Body) > 5000 {
		return in, badRequest("body must be at most 5000 characters")
	}
	atts, err := s.cleanAttachments(in.Attachments)
	in.Attachments = atts
	return in, err
}

func (s *server) createPost(w http.ResponseWriter, r *http.Request, me *User) error {
	var raw postInput
	if err := readJSON(w, r, &raw); err != nil {
		return err
	}
	in, err := s.cleanPostInput(raw)
	if err != nil {
		return err
	}
	if in.GroupID != nil {
		if role, err := s.groupRole(*in.GroupID, me.ID); err != nil || role == "" {
			return forbidden("join the group to post in it")
		}
	}
	res, err := s.db.Exec(`INSERT INTO posts(author_id, group_id, body, attachments, created_at) VALUES (?, ?, ?, ?, ?)`,
		me.ID, in.GroupID, in.Body, jsonString(in.Attachments), nowString())
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	s.broadcastPost("post.created", id)
	return s.respondPost(w, http.StatusCreated, me.ID, id)
}

// updatePost lets the author change a post's text and attachments; everyone
// who can see it gets a post.updated event.
func (s *server) updatePost(w http.ResponseWriter, r *http.Request, me *User) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var raw postInput
	if err := readJSON(w, r, &raw); err != nil {
		return err
	}
	in, err := s.cleanPostInput(raw)
	if err != nil {
		return err
	}
	o, err := s.postOwner(id)
	if err != nil {
		return err
	}
	if o.author != me.ID {
		return forbidden("only the author can edit a post")
	}
	if _, err := s.db.Exec(`UPDATE posts SET body = ?, attachments = ?, edited_at = ? WHERE id = ?`, in.Body, jsonString(in.Attachments), nowString(), id); err != nil {
		return err
	}
	s.broadcastPost("post.updated", id)
	return s.respondPost(w, http.StatusOK, me.ID, id)
}

// mayDeletePost: the author, or the admin of the group it was posted in.
func (s *server) mayDeletePost(userID int64, o postOwner) bool {
	if o.author == userID {
		return true
	}
	if o.groupID == nil {
		return false
	}
	role, _ := s.groupRole(*o.groupID, userID)
	return role == "admin"
}

func (s *server) deletePost(w http.ResponseWriter, r *http.Request, me *User) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	o, err := s.postOwner(id)
	if err != nil {
		return err
	}
	if !s.mayDeletePost(me.ID, o) {
		return forbidden("not allowed")
	}
	audience := s.postAudience(o.author, o.groupID)
	if _, err := s.db.Exec(`DELETE FROM posts WHERE id = ?`, id); err != nil {
		return err
	}
	s.hub.sendToMany(audience, postDeletedEvent{Type: "post.deleted", ID: id, GroupID: o.groupID})
	return noContent(w)
}

// ---- reactions

func validEmoji(raw string) (string, error) {
	e := strings.TrimSpace(raw)
	if e == "" || len(e) > 32 || !utf8.ValidString(e) || utf8.RuneCountInString(e) > 8 {
		return "", badRequest("invalid emoji")
	}
	return e, nil
}

// setReaction upserts the caller's single reaction on a post. Any emoji works.
func (s *server) setReaction(w http.ResponseWriter, r *http.Request, me *User) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		Emoji string `json:"emoji"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	emoji, err := validEmoji(in.Emoji)
	if err != nil {
		return err
	}
	if _, err := s.postOwner(id); err != nil {
		return err
	}
	if _, err := s.db.Exec(`INSERT INTO reactions(post_id, user_id, emoji) VALUES (?, ?, ?)
		ON CONFLICT(post_id, user_id) DO UPDATE SET emoji = excluded.emoji`, id, me.ID, emoji); err != nil {
		return err
	}
	s.broadcastPost("post.updated", id)
	return s.respondPost(w, http.StatusOK, me.ID, id)
}

func (s *server) deleteReaction(w http.ResponseWriter, r *http.Request, me *User) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(`DELETE FROM reactions WHERE post_id = ? AND user_id = ?`, id, me.ID); err != nil {
		return err
	}
	s.broadcastPost("post.updated", id)
	return s.respondPost(w, http.StatusOK, me.ID, id)
}

// ---- comments

// deletedUser stands in for the author of a deleted comment.
var deletedUser = User{Username: "deleted", DisplayName: "[deleted]"}

const commentCols = `c.id, c.post_id, c.parent_id, c.body, c.created_at, c.edited_at, c.deleted_at, ` + userCols

func scanComment(sc scanner) (Comment, error) {
	var c Comment
	var deleted *string
	err := sc.Scan(fields([]any{&c.ID, &c.PostID, &c.ParentID, &c.Body, &c.CreatedAt, &c.EditedAt, &deleted}, c.Author.fields())...)
	if deleted != nil {
		c.Deleted, c.Body, c.EditedAt, c.Author = true, "[deleted]", nil, deletedUser
	}
	return c, err
}

func (s *server) loadComment(id int64) (Comment, error) {
	return scanComment(s.db.QueryRow(`SELECT `+commentCols+` FROM comments c JOIN users u ON u.id = c.author_id WHERE c.id = ?`, id))
}

// ownComment loads a live comment the caller wrote, for editing or deleting.
func (s *server) ownComment(id, userID int64, verb string) (Comment, error) {
	c, err := s.loadComment(id)
	if err = orNotFound(err, "comment not found"); err != nil {
		return c, err
	}
	if c.Deleted {
		return c, notFound("comment not found")
	}
	if c.Author.ID != userID {
		return c, forbidden("only the author can " + verb + " a comment")
	}
	return c, nil
}

// commentAudience is everyone who can see the comment's post.
func (s *server) commentAudience(postID int64) []int64 {
	o, err := s.postOwner(postID)
	if err != nil {
		return nil
	}
	return s.postAudience(o.author, o.groupID)
}

func validCommentBody(raw string) (string, error) {
	body := strings.TrimSpace(raw)
	if !clampLen(body, 2000) {
		return "", badRequest("body must be 1-2000 characters")
	}
	return body, nil
}

func (s *server) listComments(w http.ResponseWriter, r *http.Request, me *User) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	comments, err := queryAll(s.db, scanComment, `SELECT `+commentCols+`
		FROM comments c JOIN users u ON u.id = c.author_id
		WHERE c.post_id = ? AND c.id > ? ORDER BY c.id ASC LIMIT 500`, id, queryInt(r, "after", 0))
	if err != nil {
		return err
	}
	return ok(w, comments)
}

// validReplyTarget checks that a reply goes under a live comment of the same post.
func (s *server) validReplyTarget(postID int64, parentID *int64) error {
	if parentID == nil {
		return nil
	}
	parent, err := s.loadComment(*parentID)
	if err != nil || parent.PostID != postID || parent.Deleted {
		return badRequest("cannot reply to that comment")
	}
	return nil
}

func (s *server) createComment(w http.ResponseWriter, r *http.Request, me *User) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		Body     string `json:"body"`
		ParentID *int64 `json:"parent_id"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	body, err := validCommentBody(in.Body)
	if err != nil {
		return err
	}
	o, err := s.postOwner(id)
	if err != nil {
		return err
	}
	if err := s.validReplyTarget(id, in.ParentID); err != nil {
		return err
	}
	c := Comment{PostID: id, ParentID: in.ParentID, Author: *me, Body: body, CreatedAt: nowString()}
	res, err := s.db.Exec(`INSERT INTO comments(post_id, parent_id, author_id, body, created_at) VALUES (?, ?, ?, ?, ?)`, id, c.ParentID, me.ID, c.Body, c.CreatedAt)
	if err != nil {
		return err
	}
	c.ID, _ = res.LastInsertId()
	s.hub.sendToMany(s.postAudience(o.author, o.groupID), commentEvent{Type: "comment.created", Comment: c})
	s.broadcastPost("post.updated", id)
	return created(w, c)
}

// updateComment lets the author change a comment's text.
func (s *server) updateComment(w http.ResponseWriter, r *http.Request, me *User) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		Body string `json:"body"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	body, err := validCommentBody(in.Body)
	if err != nil {
		return err
	}
	if _, err := s.ownComment(id, me.ID, "edit"); err != nil {
		return err
	}
	if _, err := s.db.Exec(`UPDATE comments SET body = ?, edited_at = ? WHERE id = ?`, body, nowString(), id); err != nil {
		return err
	}
	c, err := s.loadComment(id)
	if err != nil {
		return err
	}
	s.hub.sendToMany(s.commentAudience(c.PostID), commentEvent{Type: "comment.updated", Comment: c})
	return ok(w, c)
}

// deleteComment removes the author's comment. One that has replies is kept as
// a [deleted] placeholder so the thread below it stays readable; placeholders
// whose last reply disappears are removed too.
func (s *server) deleteComment(w http.ResponseWriter, r *http.Request, me *User) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	c, err := s.ownComment(id, me.ID, "delete")
	if err != nil {
		return err
	}
	if err := s.removeComment(c); err != nil {
		return err
	}
	s.hub.sendToMany(s.commentAudience(c.PostID), commentDeletedEvent{Type: "comment.deleted", PostID: c.PostID, ID: id})
	s.broadcastPost("post.updated", c.PostID)
	return noContent(w)
}

func (s *server) removeComment(c Comment) error {
	var replies int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM comments WHERE parent_id = ?`, c.ID).Scan(&replies); err != nil {
		return err
	}
	if replies > 0 {
		_, err := s.db.Exec(`UPDATE comments SET deleted_at = ?, body = '' WHERE id = ?`, nowString(), c.ID)
		return err
	}
	if _, err := s.db.Exec(`DELETE FROM comments WHERE id = ?`, c.ID); err != nil {
		return err
	}
	return s.pruneDeletedAncestors(c.ParentID)
}

// pruneDeletedAncestors walks up from a parent: a [deleted] placeholder with
// nothing left under it goes too.
func (s *server) pruneDeletedAncestors(parent *int64) error {
	for parent != nil {
		var deleted *string
		var next *int64
		var replies int
		err := s.db.QueryRow(`SELECT deleted_at, parent_id, (SELECT COUNT(*) FROM comments x WHERE x.parent_id = c.id) FROM comments c WHERE id = ?`, *parent).
			Scan(&deleted, &next, &replies)
		if err != nil || deleted == nil || replies > 0 {
			return nil
		}
		if _, err := s.db.Exec(`DELETE FROM comments WHERE id = ?`, *parent); err != nil {
			return err
		}
		parent = next
	}
	return nil
}
