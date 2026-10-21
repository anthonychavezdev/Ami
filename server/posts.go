package main

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"unicode/utf8"
)

type Post struct {
	ID           string         `json:"id"`
	Author       User           `json:"author"`
	GroupID      *string        `json:"group_id"`
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
	ID         string         `json:"id"`
	PostID     string         `json:"post_id"`
	ParentID   *string        `json:"parent_id"` // reply to another comment of the same post
	Author     User           `json:"author"`
	Body       string         `json:"body"`
	CreatedAt  string         `json:"created_at"`
	EditedAt   *string        `json:"edited_at"`
	Deleted    bool           `json:"deleted"`     // body and author read [deleted]; the row stays so replies keep their place
	Reactions  map[string]int `json:"reactions"`   // emoji -> count, like a post's
	MyReaction string         `json:"my_reaction"` // "" if none
}

// ---- reading posts

// postSelect expects the viewer's user id as its first bound parameter. The
// reaction counts come separately (fillReactions): one query for the whole
// page, driven by the reactions' primary key, instead of aggregating every
// reaction in the table on each read.
const postSelect = `
SELECT p.id, p.body, p.attachments, p.created_at, p.edited_at, p.group_id, COALESCE(g.name, ''),
       ` + userCols + `,
       (SELECT COUNT(*) FROM comments c WHERE c.post_id = p.id AND c.deleted_at IS NULL),
       COALESCE((SELECT emoji FROM reactions r WHERE r.post_id = p.id AND r.user_id = ?), '')
FROM posts p
JOIN users u ON u.id = p.author_id
LEFT JOIN groups g ON g.id = p.group_id
`

func scanPost(sc scanner) (Post, error) {
	var p Post
	var attachments string
	err := sc.Scan(fields([]any{&p.ID, &p.Body, &attachments, &p.CreatedAt, &p.EditedAt, &p.GroupID, &p.GroupName},
		p.Author.fields(), []any{&p.CommentCount, &p.MyReaction})...)
	if err != nil {
		return p, err
	}
	p.Reactions = map[string]int{}
	p.Attachments = decodeAttachments(attachments)
	return p, nil
}

// fillReactions puts each post's {emoji: count} in place, in one query.
func (s *server) fillReactions(posts []Post) error {
	if len(posts) == 0 {
		return nil
	}
	byID := make(map[string]*Post, len(posts))
	ids := make([]any, 0, len(posts))
	for i := range posts {
		byID[posts[i].ID] = &posts[i]
		ids = append(ids, posts[i].ID)
	}
	rows, err := s.db.Query(`SELECT post_id, emoji, COUNT(*) FROM reactions WHERE post_id IN (`+placeholders(len(ids))+`) GROUP BY post_id, emoji`, ids...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, emoji string
		var n int
		if err := rows.Scan(&id, &emoji, &n); err != nil {
			return err
		}
		if p := byID[id]; p != nil {
			p.Reactions[emoji] = n
		}
	}
	return rows.Err()
}

// decodeAttachments turns a stored JSON column into a (never nil) slice.
func decodeAttachments(raw string) []Attachment {
	out := []Attachment{}
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}

// queryPosts runs postSelect + tail for viewer, binding args after the viewer id.
func (s *server) queryPosts(viewer string, tail string, args ...any) ([]Post, error) {
	posts, err := queryAll(s.db, scanPost, postSelect+tail, append([]any{viewer}, args...)...)
	if err != nil {
		return nil, err
	}
	return posts, s.fillReactions(posts)
}

func (s *server) getPost(viewer, id string) (Post, error) {
	p, err := scanPost(s.db.QueryRow(postSelect+`WHERE p.id = ?`, viewer, id))
	if err != nil {
		return p, err
	}
	posts := []Post{p}
	return posts[0], s.fillReactions(posts)
}

func (s *server) respondPost(w http.ResponseWriter, code int, viewer, id string) error {
	p, err := s.getPost(viewer, id)
	if err != nil {
		return orNotFound(err, "post not found")
	}
	writeJSON(w, code, p)
	return nil
}

// postOwner is who wrote a post and which group it was posted in.
type postOwner struct {
	author  string
	groupID *string
}

func (s *server) postOwner(id string) (postOwner, error) {
	var o postOwner
	err := s.db.QueryRow(`SELECT author_id, group_id FROM posts WHERE id = ?`, id).Scan(&o.author, &o.groupID)
	return o, orNotFound(err, "post not found")
}

// mayViewPost is the one place a post's visibility is decided: anyone signed
// in sees a profile post or a public group's; a private group's posts are for
// its members, and to anyone else the post is not found.
func (s *server) mayViewPost(viewer, postID string) error {
	o, err := s.postOwner(postID)
	if err != nil {
		return err
	}
	if o.groupID == nil {
		return nil
	}
	ok, err := s.groupVisible(*o.groupID, viewer)
	if err != nil {
		return err
	}
	if !ok {
		return notFound("post not found")
	}
	return nil
}

// viewingPost and viewingComment guard the routes on a post or on a comment
// (through its post) with mayViewPost, so no handler has to remember.
func (s *server) viewingPost(h authedHandler) authedHandler {
	return func(w http.ResponseWriter, r *http.Request, me *User) error {
		if err := s.mayViewPost(me.ID, r.PathValue("id")); err != nil {
			return err
		}
		return h(w, r, me)
	}
}

func (s *server) viewingComment(h authedHandler) authedHandler {
	return func(w http.ResponseWriter, r *http.Request, me *User) error {
		var postID string
		err := s.db.QueryRow(`SELECT post_id FROM comments WHERE id = ?`, r.PathValue("id")).Scan(&postID)
		if err = orNotFound(err, "comment not found"); err != nil {
			return err
		}
		if err := s.mayViewPost(me.ID, postID); err != nil {
			return err
		}
		return h(w, r, me)
	}
}

// postAudience lists who sees a post in their feed: the author and their
// friends for profile posts, every member for group posts.
func (s *server) postAudience(authorID string, groupID *string) []string {
	if groupID != nil {
		return append([]string{authorID}, without(s.groupMemberIDs(*groupID), authorID)...)
	}
	return append([]string{authorID}, s.friendIDs(authorID)...)
}

// broadcastPost pushes a post's current state to everyone who sees it, each
// with their own reaction filled in, so feeds update without a refresh. The
// post is loaded once; who reacted with what is one more query, so the
// audience's size does not multiply the work.
func (s *server) broadcastPost(typ string, id string) {
	base, err := s.getPost("", id)
	if err != nil {
		return
	}
	mine := map[string]string{} // user -> their reaction
	rows, err := s.db.Query(`SELECT user_id, emoji FROM reactions WHERE post_id = ?`, id)
	if err != nil {
		return
	}
	for rows.Next() {
		var uid, emoji string
		if rows.Scan(&uid, &emoji) == nil {
			mine[uid] = emoji
		}
	}
	rows.Close()
	for _, uid := range s.postAudience(base.Author.ID, base.GroupID) {
		if !s.hub.online(uid) {
			continue
		}
		p := base
		p.MyReaction = mine[uid]
		s.hub.sendTo(uid, postEvent{Type: typ, Post: p})
	}
}

// feed: my own posts, friends' profile posts, and posts in groups I belong to.
func (s *server) feed(w http.ResponseWriter, r *http.Request, me *User) error {
	before, limit := pagination(r)
	posts, err := s.queryPosts(me.ID, `
		WHERE p.id < ? AND (
			(p.group_id IS NULL AND (p.author_id = ? OR p.author_id IN (
				SELECT CASE WHEN from_id = ? THEN to_id ELSE from_id END FROM friendships
				WHERE status = 'accepted' AND (from_id = ? OR to_id = ?))))
			OR p.group_id IN (SELECT group_id FROM group_members WHERE user_id = ?))
		ORDER BY p.id DESC LIMIT ?`, before, me.ID, me.ID, me.ID, me.ID, me.ID, limit)
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
	GroupID     *string      `json:"group_id"`
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
	id := newID()
	if _, err := s.db.Exec(`INSERT INTO posts(id, author_id, group_id, body, attachments, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		id, me.ID, in.GroupID, in.Body, jsonString(in.Attachments), nowString()); err != nil {
		return err
	}
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
func (s *server) mayDeletePost(userID string, o postOwner) bool {
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
		ON DUPLICATE KEY UPDATE emoji = VALUES(emoji)`, id, me.ID, emoji); err != nil {
		return err
	}
	s.broadcastPost("post.updated", id)
	return s.respondPost(w, http.StatusOK, me.ID, id)
}

// ReactionGroup is everyone who reacted to a post or comment with one emoji.
type ReactionGroup struct {
	Emoji string `json:"emoji"`
	Users []User `json:"users"`
}

// listReactions answers who reacted to a post with what: one group per
// emoji, the most used first, people by display name. Posts are visible to
// any signed-in user, so their reactions are too.
func (s *server) listReactions(w http.ResponseWriter, r *http.Request, me *User) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	if _, err := s.postOwner(id); err != nil {
		return err
	}
	return s.respondReactionGroups(w, `SELECT r.emoji, `+userCols+` FROM reactions r JOIN users u ON u.id = r.user_id WHERE r.post_id = ? ORDER BY r.emoji, u.display_name`, id)
}

// respondReactionGroups groups the (emoji, user) rows of a query into
// ReactionGroups, the most used emoji first.
func (s *server) respondReactionGroups(w http.ResponseWriter, query string, id string) error {
	type reaction struct {
		emoji string
		user  User
	}
	rows, err := queryAll(s.db, func(sc scanner) (reaction, error) {
		var x reaction
		return x, sc.Scan(fields([]any{&x.emoji}, x.user.fields())...)
	}, query, id)
	if err != nil {
		return err
	}
	groups := []ReactionGroup{}
	index := map[string]int{}
	for _, x := range rows {
		i, ok := index[x.emoji]
		if !ok {
			i = len(groups)
			index[x.emoji] = i
			groups = append(groups, ReactionGroup{Emoji: x.emoji, Users: []User{}})
		}
		groups[i].Users = append(groups[i].Users, x.user)
	}
	sort.SliceStable(groups, func(i, j int) bool { return len(groups[i].Users) > len(groups[j].Users) })
	return ok(w, groups)
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

// commentSelect expects the viewer's user id as its first bound parameter,
// for my_reaction; the reaction counts come separately (fillCommentReactions),
// as a post's do.
const commentSelect = `SELECT c.id, c.post_id, c.parent_id, c.body, c.created_at, c.edited_at, c.deleted_at, ` + userCols + `,
	COALESCE((SELECT emoji FROM comment_reactions r WHERE r.comment_id = c.id AND r.user_id = ?), '')
	FROM comments c JOIN users u ON u.id = c.author_id `

func scanComment(sc scanner) (Comment, error) {
	var c Comment
	var deleted *string
	err := sc.Scan(fields([]any{&c.ID, &c.PostID, &c.ParentID, &c.Body, &c.CreatedAt, &c.EditedAt, &deleted}, c.Author.fields(), []any{&c.MyReaction})...)
	c.Reactions = map[string]int{}
	if deleted != nil {
		c.Deleted, c.Body, c.EditedAt, c.Author, c.MyReaction = true, "[deleted]", nil, deletedUser, ""
	}
	return c, err
}

// fillCommentReactions puts each live comment's {emoji: count} in place, in one query.
func (s *server) fillCommentReactions(comments []Comment) error {
	byID := map[string]*Comment{}
	ids := []any{}
	for i := range comments {
		if !comments[i].Deleted {
			byID[comments[i].ID] = &comments[i]
			ids = append(ids, comments[i].ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	rows, err := s.db.Query(`SELECT comment_id, emoji, COUNT(*) FROM comment_reactions WHERE comment_id IN (`+placeholders(len(ids))+`) GROUP BY comment_id, emoji`, ids...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, emoji string
		var n int
		if err := rows.Scan(&id, &emoji, &n); err != nil {
			return err
		}
		if c := byID[id]; c != nil {
			c.Reactions[emoji] = n
		}
	}
	return rows.Err()
}

// loadComment reads one comment as viewer sees it ("" for nobody in particular).
func (s *server) loadComment(viewer, id string) (Comment, error) {
	c, err := scanComment(s.db.QueryRow(commentSelect+`WHERE c.id = ?`, viewer, id))
	if err != nil {
		return c, err
	}
	comments := []Comment{c}
	return comments[0], s.fillCommentReactions(comments)
}

// liveComment loads a comment that is not a [deleted] placeholder, or 404.
func (s *server) liveComment(viewer, id string) (Comment, error) {
	c, err := s.loadComment(viewer, id)
	if err = orNotFound(err, "comment not found"); err != nil {
		return c, err
	}
	if c.Deleted {
		return c, notFound("comment not found")
	}
	return c, nil
}

// ownComment loads a live comment the caller wrote, for editing or deleting.
func (s *server) ownComment(id, userID string, verb string) (Comment, error) {
	c, err := s.loadComment(userID, id)
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
func (s *server) commentAudience(postID string) []string {
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
	comments, err := queryAll(s.db, scanComment, commentSelect+`WHERE c.post_id = ? AND c.id > ? ORDER BY c.id ASC LIMIT 500`, me.ID, id, queryID(r, "after", ""))
	if err != nil {
		return err
	}
	if err := s.fillCommentReactions(comments); err != nil {
		return err
	}
	return ok(w, comments)
}

// validReplyTarget checks that a reply goes under a live comment of the same post.
func (s *server) validReplyTarget(postID string, parentID *string) error {
	if parentID == nil {
		return nil
	}
	parent, err := s.loadComment("", *parentID)
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
		Body     string  `json:"body"`
		ParentID *string `json:"parent_id"`
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
	c := Comment{ID: newID(), PostID: id, ParentID: in.ParentID, Author: *me, Body: body, CreatedAt: nowString(), Reactions: map[string]int{}}
	if _, err := s.db.Exec(`INSERT INTO comments(id, post_id, parent_id, author_id, body, created_at) VALUES (?, ?, ?, ?, ?, ?)`, c.ID, id, c.ParentID, me.ID, c.Body, c.CreatedAt); err != nil {
		return err
	}
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
	s.broadcastComment(id)
	return s.respondComment(w, me.ID, id)
}

func (s *server) respondComment(w http.ResponseWriter, viewer, id string) error {
	c, err := s.loadComment(viewer, id)
	if err != nil {
		return orNotFound(err, "comment not found")
	}
	return ok(w, c)
}

// broadcastComment sends a comment's current state to everyone who sees its
// post, each with their own reaction filled in, as broadcastPost does.
func (s *server) broadcastComment(id string) {
	base, err := s.loadComment("", id)
	if err != nil {
		return
	}
	mine := map[string]string{}
	rows, err := s.db.Query(`SELECT user_id, emoji FROM comment_reactions WHERE comment_id = ?`, id)
	if err != nil {
		return
	}
	for rows.Next() {
		var uid, emoji string
		if rows.Scan(&uid, &emoji) == nil {
			mine[uid] = emoji
		}
	}
	rows.Close()
	for _, uid := range s.commentAudience(base.PostID) {
		if !s.hub.online(uid) {
			continue
		}
		c := base
		c.MyReaction = mine[uid]
		s.hub.sendTo(uid, commentEvent{Type: "comment.updated", Comment: c})
	}
}

// ---- reactions on comments: the same shape as a post's

func (s *server) setCommentReaction(w http.ResponseWriter, r *http.Request, me *User) error {
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
	if _, err := s.liveComment("", id); err != nil {
		return err
	}
	if _, err := s.db.Exec(`INSERT INTO comment_reactions(comment_id, user_id, emoji) VALUES (?, ?, ?)
		ON DUPLICATE KEY UPDATE emoji = VALUES(emoji)`, id, me.ID, emoji); err != nil {
		return err
	}
	s.broadcastComment(id)
	return s.respondComment(w, me.ID, id)
}

func (s *server) deleteCommentReaction(w http.ResponseWriter, r *http.Request, me *User) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	if _, err := s.liveComment("", id); err != nil {
		return err
	}
	if _, err := s.db.Exec(`DELETE FROM comment_reactions WHERE comment_id = ? AND user_id = ?`, id, me.ID); err != nil {
		return err
	}
	s.broadcastComment(id)
	return s.respondComment(w, me.ID, id)
}

// listCommentReactions answers who reacted to a comment with what.
func (s *server) listCommentReactions(w http.ResponseWriter, r *http.Request, me *User) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	if _, err := s.liveComment("", id); err != nil {
		return err
	}
	return s.respondReactionGroups(w, `SELECT r.emoji, `+userCols+` FROM comment_reactions r JOIN users u ON u.id = r.user_id WHERE r.comment_id = ? ORDER BY r.emoji, u.display_name`, id)
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
func (s *server) pruneDeletedAncestors(parent *string) error {
	for parent != nil {
		var deleted *string
		var next *string
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
