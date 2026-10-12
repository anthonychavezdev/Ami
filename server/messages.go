package main

import (
	"database/sql"
	"errors"
	"net/http"
	"sort"
	"strings"
)

// Message is a direct message (RecipientID set) or a group chat message
// (GroupID set), never both.
type Message struct {
	ID          int64        `json:"id"`
	SenderID    int64        `json:"sender_id"`
	RecipientID *int64       `json:"recipient_id"`
	GroupID     *int64       `json:"group_id"`
	Sender      *User        `json:"sender,omitempty"` // group chats: several people write, so each message names its author
	Body        string       `json:"body"`
	Attachments []Attachment `json:"attachments"`
	CreatedAt   string       `json:"created_at"`
	EditedAt    *string      `json:"edited_at"` // null until the sender edits it
}

// Conversation is one row of the Messages tab: a direct chat or a group chat.
type Conversation struct {
	User        *User    `json:"user,omitempty"`
	Group       *Group   `json:"group,omitempty"`
	LastMessage *Message `json:"last_message"` // null for a group nobody has written in yet
	Unread      int      `json:"unread"`
}

// ---- which chat

// chat is where a message lives: a direct chat with user peer, or group group.
type chat struct {
	peer  int64
	group int64
}

func (c chat) isGroup() bool { return c.group != 0 }

// filter selects the chat's messages (alias m); ?1 is the viewer, ?2 the chat.
func (c chat) filter() (string, int64) {
	if c.isGroup() {
		return "m.group_id = ?2", c.group
	}
	return "m.group_id IS NULL AND ((m.sender_id = ?1 AND m.recipient_id = ?2) OR (m.sender_id = ?2 AND m.recipient_id = ?1))", c.peer
}

// contains is whether a message belongs to the chat.
func (c chat) contains(m Message) bool {
	if c.isGroup() {
		return m.GroupID != nil && *m.GroupID == c.group
	}
	return m.GroupID == nil && m.RecipientID != nil && (*m.RecipientID == c.peer || m.SenderID == c.peer)
}

// readEvent is the messages.read event for this chat with the user's new total.
func (c chat) readEvent(unread int) messagesReadEvent {
	e := messagesReadEvent{Type: "messages.read", Unread: unread}
	if c.isGroup() {
		e.GroupID = &c.group
	} else {
		e.PeerID = &c.peer
	}
	return e
}

// A chatHandler serves one chat action; the routes below say how the chat is
// found: /api/messages/{id} is a direct chat with user {id}, and
// /api/groups/{id}/messages is group {id}'s chat, for members only.
type chatHandler func(w http.ResponseWriter, r *http.Request, me *User, c chat) error

func (s *server) directChat(h chatHandler) authedHandler {
	return func(w http.ResponseWriter, r *http.Request, me *User) error {
		other, err := pathID(r, "id")
		if err != nil {
			return err
		}
		return h(w, r, me, chat{peer: other})
	}
}

func (s *server) groupChat(h chatHandler) authedHandler {
	return func(w http.ResponseWriter, r *http.Request, me *User) error {
		id, err := pathID(r, "id")
		if err != nil {
			return err
		}
		role, err := s.groupRole(id, me.ID)
		if err != nil {
			return err
		}
		if role == "" {
			return forbidden("only members can use the group chat")
		}
		return h(w, r, me, chat{group: id})
	}
}

// ---- reading messages

const messageCols = "m.id, m.sender_id, m.recipient_id, m.group_id, m.body, m.attachments, m.created_at, m.edited_at"

// fields are the scan targets for messageCols, in that order; the attachments
// column lands in atts as JSON.
func (m *Message) fields(atts *string) []any {
	return []any{&m.ID, &m.SenderID, &m.RecipientID, &m.GroupID, &m.Body, atts, &m.CreatedAt, &m.EditedAt}
}

// scanMessage reads messageCols followed by the sender's userCols; the sender
// is kept on group messages only (direct chats know who the other person is).
func scanMessage(sc scanner) (Message, error) {
	var m Message
	var u User
	var atts string
	if err := sc.Scan(fields(m.fields(&atts), u.fields())...); err != nil {
		return m, err
	}
	m.Attachments = decodeAttachments(atts)
	if m.GroupID != nil {
		m.Sender = &u
	}
	return m, nil
}

func (s *server) loadMessage(id int64) (Message, error) {
	return scanMessage(s.db.QueryRow(`SELECT `+messageCols+`, `+userCols+` FROM messages m JOIN users u ON u.id = m.sender_id WHERE m.id = ?`, id))
}

// listChat returns the newest messages of a chat, oldest first within the page.
func (s *server) listChat(w http.ResponseWriter, r *http.Request, me *User, c chat) error {
	before, limit := pagination(r)
	if limit == 30 {
		limit = 50
	}
	where, target := c.filter()
	messages, err := queryAll(s.db, scanMessage, `SELECT `+messageCols+`, `+userCols+` FROM (
			SELECT * FROM messages m WHERE `+where+` AND m.id < ?3 ORDER BY m.id DESC LIMIT ?4) m
		JOIN users u ON u.id = m.sender_id ORDER BY m.id ASC`, me.ID, target, before, limit)
	if err != nil {
		return err
	}
	return ok(w, messages)
}

// chatMessage loads message {mid} if it belongs to the chat.
func (s *server) chatMessage(r *http.Request, c chat) (Message, error) {
	mid, err := pathID(r, "mid")
	if err != nil {
		return Message{}, err
	}
	m, err := s.loadMessage(mid)
	if err = orNotFound(err, "message not found"); err != nil {
		return m, err
	}
	if !c.contains(m) {
		return m, notFound("message not found")
	}
	return m, nil
}

// chatAudience is everyone who sees a message: the group's members, or both
// parties of a direct chat.
func (s *server) chatAudience(m Message) []int64 {
	switch {
	case m.GroupID != nil:
		return s.groupMemberIDs(*m.GroupID)
	case m.RecipientID != nil:
		return []int64{m.SenderID, *m.RecipientID}
	default:
		return []int64{m.SenderID}
	}
}

// ---- writing messages

type messageInput struct {
	Body        string       `json:"body"`
	Attachments []Attachment `json:"attachments"`
}

// cleanMessage trims the text and checks it and the attachments.
func (s *server) cleanMessage(body string, atts []Attachment) (string, []Attachment, error) {
	body = strings.TrimSpace(body)
	clean, err := s.cleanAttachments(atts)
	if err != nil {
		return body, nil, err
	}
	if body == "" && len(clean) == 0 {
		return body, clean, badRequest("a message needs text or an attachment")
	}
	if len(body) > 4000 {
		return body, clean, badRequest("body must be at most 4000 characters")
	}
	return body, clean, nil
}

// nullID stores 0 as NULL.
func nullID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}

// sendChat stores a message (text, pictures/videos uploaded via /api/media, or
// both) and pushes it to everyone in the chat over the WebSocket; people with
// no connection get an OS notification.
func (s *server) sendChat(w http.ResponseWriter, r *http.Request, me *User, c chat) error {
	var in messageInput
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	body, atts, err := s.cleanMessage(in.Body, in.Attachments)
	if err != nil {
		return err
	}
	if !c.isGroup() {
		if friends, err := s.areFriends(me.ID, c.peer); err != nil || !friends {
			return forbidden("you can only message friends")
		}
	}
	res, err := s.db.Exec(`INSERT INTO messages(sender_id, recipient_id, group_id, body, attachments, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		me.ID, nullID(c.peer), nullID(c.group), body, jsonString(atts), nowString())
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	m, err := s.loadMessage(id)
	if err != nil {
		return err
	}
	s.deliverMessage(me, m)
	return created(w, m)
}

// deliverMessage sends a new message to everyone in its chat with their own
// unread total; the sender's other devices get a plain copy, and people with
// no connection are pushed.
func (s *server) deliverMessage(from *User, m Message) {
	event := messageEvent{Type: "message", Message: m, Sender: from}
	if m.GroupID != nil {
		g := s.groupSummary(*m.GroupID)
		event.Group = &g
	}
	for _, uid := range s.chatAudience(m) {
		if uid == from.ID {
			s.hub.sendTo(uid, event)
			continue
		}
		unread, _ := s.unreadMessages(uid)
		theirs := event
		theirs.Unread = &unread
		if !s.hub.sendTo(uid, theirs) {
			go s.pushMessage(uid, from, m, unread)
		}
	}
}

// updateChat edits the text of one of the caller's messages; everyone in the
// chat gets a message.updated event.
func (s *server) updateChat(w http.ResponseWriter, r *http.Request, me *User, c chat) error {
	var in struct {
		Body string `json:"body"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	m, err := s.chatMessage(r, c)
	if err != nil {
		return err
	}
	if m.SenderID != me.ID {
		return forbidden("only the sender can edit a message")
	}
	body, _, err := s.cleanMessage(in.Body, m.Attachments)
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(`UPDATE messages SET body = ?, edited_at = ? WHERE id = ?`, body, nowString(), m.ID); err != nil {
		return err
	}
	if m, err = s.loadMessage(m.ID); err != nil {
		return err
	}
	s.hub.sendToMany(s.chatAudience(m), messageUpdatedEvent{Type: "message.updated", Message: m})
	return ok(w, m)
}

// deleteChat removes a message for everyone in the chat: the sender's own, or,
// in a group, any message when the caller is the admin. Badges are refreshed
// by the message.deleted event.
func (s *server) deleteChat(w http.ResponseWriter, r *http.Request, me *User, c chat) error {
	m, err := s.chatMessage(r, c)
	if err != nil {
		return err
	}
	if !s.mayDeleteMessage(me.ID, m, c) {
		return forbidden("only the sender can delete a message")
	}
	if _, err := s.db.Exec(`DELETE FROM messages WHERE id = ?`, m.ID); err != nil {
		return err
	}
	if err := s.pullBackReadMarkers(m, c); err != nil {
		return err
	}
	for _, uid := range s.chatAudience(m) {
		unread, _ := s.unreadMessages(uid)
		s.hub.sendTo(uid, messageDeletedEvent{Type: "message.deleted", ID: m.ID, SenderID: m.SenderID, RecipientID: m.RecipientID, GroupID: m.GroupID, Unread: unread})
	}
	return noContent(w)
}

func (s *server) mayDeleteMessage(userID int64, m Message, c chat) bool {
	if m.SenderID == userID {
		return true
	}
	if !c.isGroup() {
		return false
	}
	role, _ := s.groupRole(c.group, userID)
	return role == "admin"
}

// pullBackReadMarkers: SQLite hands the highest id out again after a delete,
// so a read marker sitting at or past the deleted message is pulled back to
// what remains; a new message reusing the id then still counts as unread.
func (s *server) pullBackReadMarkers(m Message, c chat) error {
	if c.isGroup() {
		_, err := s.db.Exec(`UPDATE group_reads SET last_read_id = (SELECT COALESCE(MAX(id), 0) FROM messages WHERE group_id = ?1)
			WHERE group_id = ?1 AND last_read_id >= ?2`, c.group, m.ID)
		return err
	}
	_, err := s.db.Exec(`UPDATE conversation_reads SET last_read_id = (SELECT COALESCE(MAX(id), 0) FROM messages WHERE group_id IS NULL AND sender_id = ?1 AND recipient_id = ?2)
		WHERE user_id = ?2 AND peer_id = ?1 AND last_read_id >= ?3`, m.SenderID, *m.RecipientID, m.ID)
	return err
}

// ---- unread counts

// unreadMessages counts messages to a user past each chat's read marker:
// direct messages from each friend, and group messages from other people.
func (s *server) unreadMessages(userID int64) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT
		(SELECT COUNT(*) FROM messages x WHERE x.group_id IS NULL AND x.recipient_id = ?1
			AND x.id > COALESCE((SELECT last_read_id FROM conversation_reads r WHERE r.user_id = ?1 AND r.peer_id = x.sender_id), 0))
		+ (SELECT COUNT(*) FROM messages x JOIN group_members gm ON gm.group_id = x.group_id AND gm.user_id = ?1
			WHERE x.sender_id != ?1
			AND x.id > COALESCE((SELECT last_read_id FROM group_reads r WHERE r.user_id = ?1 AND r.group_id = x.group_id), 0))`, userID).Scan(&n)
	return n, err
}

// groupUnread counts a group's messages from others past the user's read marker.
func (s *server) groupUnread(userID, groupID int64) int {
	var n int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM messages x WHERE x.group_id = ?2 AND x.sender_id != ?1
		AND x.id > COALESCE((SELECT last_read_id FROM group_reads r WHERE r.user_id = ?1 AND r.group_id = ?2), 0)`, userID, groupID).Scan(&n)
	return n
}

// readChat moves the chat's read marker to its latest message and tells the
// user's other devices so their badges drop too.
func (s *server) readChat(w http.ResponseWriter, r *http.Request, me *User, c chat) error {
	if err := s.moveReadMarker(me.ID, c); err != nil {
		return err
	}
	unread, err := s.unreadMessages(me.ID)
	if err != nil {
		return err
	}
	s.hub.sendTo(me.ID, c.readEvent(unread))
	return ok(w, unreadCount{unread})
}

func (s *server) moveReadMarker(userID int64, c chat) error {
	if c.isGroup() {
		_, err := s.db.Exec(`INSERT INTO group_reads(user_id, group_id, last_read_id)
			VALUES (?1, ?2, COALESCE((SELECT MAX(id) FROM messages WHERE group_id = ?2), 0))
			ON CONFLICT(user_id, group_id) DO UPDATE SET last_read_id = excluded.last_read_id`, userID, c.group)
		return err
	}
	_, err := s.db.Exec(`INSERT INTO conversation_reads(user_id, peer_id, last_read_id)
		VALUES (?1, ?2, COALESCE((SELECT MAX(id) FROM messages WHERE group_id IS NULL AND sender_id = ?2 AND recipient_id = ?1), 0))
		ON CONFLICT(user_id, peer_id) DO UPDATE SET last_read_id = excluded.last_read_id`, userID, c.peer)
	return err
}

// ---- the Messages tab

// listConversations returns every direct chat with messages and every group
// the user belongs to, most recent activity first.
func (s *server) listConversations(w http.ResponseWriter, r *http.Request, me *User) error {
	direct, err := s.directConversations(me.ID)
	if err != nil {
		return err
	}
	groups, err := s.groupConversations(me.ID)
	if err != nil {
		return err
	}
	return ok(w, sortConversations(append(direct, groups...)))
}

// directConversations: the newest message with each friend, with the unread count.
func (s *server) directConversations(userID int64) ([]Conversation, error) {
	scan := func(sc scanner) (Conversation, error) {
		var c Conversation
		var m Message
		var u User
		var atts string
		err := sc.Scan(fields(m.fields(&atts), u.fields(), []any{&c.Unread})...)
		m.Attachments = decodeAttachments(atts)
		c.User, c.LastMessage = &u, &m
		return c, err
	}
	return queryAll(s.db, scan, `
		SELECT `+messageCols+`, `+userCols+`,
			(SELECT COUNT(*) FROM messages x WHERE x.group_id IS NULL AND x.sender_id = u.id AND x.recipient_id = ?1
				AND x.id > COALESCE((SELECT last_read_id FROM conversation_reads r WHERE r.user_id = ?1 AND r.peer_id = u.id), 0))
		FROM messages m
		JOIN users u ON u.id = CASE WHEN m.sender_id = ?1 THEN m.recipient_id ELSE m.sender_id END
		WHERE m.id IN (
			SELECT MAX(id) FROM messages WHERE group_id IS NULL AND (sender_id = ?1 OR recipient_id = ?1)
			GROUP BY CASE WHEN sender_id = ?1 THEN recipient_id ELSE sender_id END)`, userID)
}

// groupConversations: every group the user is in, with its newest message if any.
func (s *server) groupConversations(userID int64) ([]Conversation, error) {
	groups, err := s.queryGroups(userID, `WHERE g.id IN (SELECT group_id FROM group_members WHERE user_id = ?1)`)
	if err != nil {
		return nil, err
	}
	out := make([]Conversation, 0, len(groups))
	for i := range groups {
		g := &groups[i]
		c := Conversation{Group: g, Unread: s.groupUnread(userID, g.ID)}
		m, err := scanMessage(s.db.QueryRow(`SELECT `+messageCols+`, `+userCols+` FROM messages m JOIN users u ON u.id = m.sender_id
			WHERE m.group_id = ? ORDER BY m.id DESC LIMIT 1`, g.ID))
		switch {
		case err == nil:
			c.LastMessage = &m
		case !errors.Is(err, sql.ErrNoRows):
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

// sortConversations: most recent message first; groups nobody has written in
// go last, by name.
func sortConversations(list []Conversation) []Conversation {
	sort.SliceStable(list, func(i, j int) bool {
		a, b := list[i].LastMessage, list[j].LastMessage
		switch {
		case a != nil && b != nil:
			return a.CreatedAt > b.CreatedAt
		case a != nil || b != nil:
			return a != nil
		default:
			return list[i].Group.Name < list[j].Group.Name
		}
	})
	return list
}
