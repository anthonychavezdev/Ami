package main

import (
	"database/sql"
	"encoding/json"
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

const messageCols = "m.id, m.sender_id, m.recipient_id, m.group_id, m.body, m.attachments, m.created_at, m.edited_at"

// decodeAttachments turns the stored JSON column into a (never nil) slice.
func decodeAttachments(raw string) []Attachment {
	out := []Attachment{}
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}

// scanMessage reads messageCols followed by the sender's userCols; the sender
// is kept on group messages only (direct chats know who the other person is).
func scanMessage(sc interface{ Scan(...any) error }) (Message, error) {
	var m Message
	var u User
	var atts string
	if err := sc.Scan(&m.ID, &m.SenderID, &m.RecipientID, &m.GroupID, &m.Body, &atts, &m.CreatedAt, &m.EditedAt,
		&u.ID, &u.Username, &u.DisplayName, &u.CreatedAt, &u.AvatarURL, &u.Pronouns, &u.AvatarCrop); err != nil {
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

// chatAudience is everyone who sees a message: the group's members, or both
// parties of a direct chat.
func (s *server) chatAudience(m Message) []int64 {
	if m.GroupID == nil {
		if m.RecipientID == nil {
			return []int64{m.SenderID}
		}
		return []int64{m.SenderID, *m.RecipientID}
	}
	return s.groupMemberIDs(*m.GroupID)
}

func (s *server) groupMemberIDs(groupID int64) []int64 {
	rows, err := s.db.Query(`SELECT user_id FROM group_members WHERE group_id = ?`, groupID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

// directChat reads the {id} of the direct-message routes.
func (s *server) directChat(w http.ResponseWriter, r *http.Request) (chat, bool) {
	other, ok := pathID(w, r, "id")
	return chat{peer: other}, ok
}

// groupChat reads the {id} of the group chat routes and admits members only.
func (s *server) groupChat(w http.ResponseWriter, r *http.Request, me *User) (chat, bool) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return chat{}, false
	}
	role, err := s.groupRole(id, me.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return chat{}, false
	}
	if role == "" {
		writeError(w, http.StatusForbidden, "only members can use the group chat")
		return chat{}, false
	}
	return chat{group: id}, true
}

func (s *server) listMessages(w http.ResponseWriter, r *http.Request, me *User) {
	if c, ok := s.directChat(w, r); ok {
		s.listChat(w, r, me, c)
	}
}

func (s *server) listGroupMessages(w http.ResponseWriter, r *http.Request, me *User) {
	if c, ok := s.groupChat(w, r, me); ok {
		s.listChat(w, r, me, c)
	}
}

func (s *server) sendMessage(w http.ResponseWriter, r *http.Request, me *User) {
	if c, ok := s.directChat(w, r); ok {
		s.sendChat(w, r, me, c)
	}
}

func (s *server) sendGroupMessage(w http.ResponseWriter, r *http.Request, me *User) {
	if c, ok := s.groupChat(w, r, me); ok {
		s.sendChat(w, r, me, c)
	}
}

func (s *server) markRead(w http.ResponseWriter, r *http.Request, me *User) {
	if c, ok := s.directChat(w, r); ok {
		s.readChat(w, me, c)
	}
}

func (s *server) markGroupRead(w http.ResponseWriter, r *http.Request, me *User) {
	if c, ok := s.groupChat(w, r, me); ok {
		s.readChat(w, me, c)
	}
}

func (s *server) updateMessage(w http.ResponseWriter, r *http.Request, me *User) {
	if c, ok := s.directChat(w, r); ok {
		s.updateChat(w, r, me, c)
	}
}

func (s *server) updateGroupMessage(w http.ResponseWriter, r *http.Request, me *User) {
	if c, ok := s.groupChat(w, r, me); ok {
		s.updateChat(w, r, me, c)
	}
}

func (s *server) deleteMessage(w http.ResponseWriter, r *http.Request, me *User) {
	if c, ok := s.directChat(w, r); ok {
		s.deleteChat(w, r, me, c)
	}
}

func (s *server) deleteGroupMessage(w http.ResponseWriter, r *http.Request, me *User) {
	if c, ok := s.groupChat(w, r, me); ok {
		s.deleteChat(w, r, me, c)
	}
}

// listChat returns the newest messages of a chat, oldest first within the page.
func (s *server) listChat(w http.ResponseWriter, r *http.Request, me *User, c chat) {
	before, limit := pagination(r)
	if limit == 30 {
		limit = 50
	}
	where, target := c.filter()
	rows, err := s.db.Query(`SELECT `+messageCols+`, `+userCols+` FROM (
			SELECT * FROM messages m WHERE `+where+` AND m.id < ?3 ORDER BY m.id DESC LIMIT ?4) m
		JOIN users u ON u.id = m.sender_id ORDER BY m.id ASC`, me.ID, target, before, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []Message{}
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, m)
	}
	writeJSON(w, http.StatusOK, out)
}

// validMessage checks the text and attachments of a new or edited message.
func (s *server) validMessage(w http.ResponseWriter, body string, atts []Attachment) bool {
	if body == "" && len(atts) == 0 {
		writeError(w, http.StatusBadRequest, "a message needs text or an attachment")
		return false
	}
	if len(body) > 4000 {
		writeError(w, http.StatusBadRequest, "body must be at most 4000 characters")
		return false
	}
	if err := s.validateAttachments(atts); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return false
	}
	return true
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
func (s *server) sendChat(w http.ResponseWriter, r *http.Request, me *User, c chat) {
	var in struct {
		Body        string       `json:"body"`
		Attachments []Attachment `json:"attachments"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	in.Body = strings.TrimSpace(in.Body)
	if in.Attachments == nil {
		in.Attachments = []Attachment{}
	}
	if !s.validMessage(w, in.Body, in.Attachments) {
		return
	}
	if !c.isGroup() {
		if friends, err := s.areFriends(me.ID, c.peer); err != nil || !friends {
			writeError(w, http.StatusForbidden, "you can only message friends")
			return
		}
	}
	attachments, _ := json.Marshal(in.Attachments)
	res, err := s.db.Exec(`INSERT INTO messages(sender_id, recipient_id, group_id, body, attachments, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		me.ID, nullID(c.peer), nullID(c.group), in.Body, string(attachments), nowString())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	id, _ := res.LastInsertId()
	m, err := s.loadMessage(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	event := map[string]any{"type": "message", "message": m, "sender": me}
	if m.GroupID != nil {
		event["group"] = s.groupSummary(*m.GroupID)
	}
	for _, uid := range s.chatAudience(m) {
		if uid == me.ID {
			s.hub.sendTo(uid, event) // the sender's other devices
			continue
		}
		unread, _ := s.unreadMessages(uid)
		ev := make(map[string]any, len(event)+1)
		for k, v := range event {
			ev[k] = v
		}
		ev["unread"] = unread
		if !s.hub.sendTo(uid, ev) {
			go s.pushMessage(uid, me, m, unread) // nothing connected: notify through the OS
		}
	}
	writeJSON(w, http.StatusCreated, m)
}

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
func (s *server) readChat(w http.ResponseWriter, me *User, c chat) {
	var err error
	event := map[string]any{"type": "messages.read"}
	if c.isGroup() {
		_, err = s.db.Exec(`INSERT INTO group_reads(user_id, group_id, last_read_id)
			VALUES (?1, ?2, COALESCE((SELECT MAX(id) FROM messages WHERE group_id = ?2), 0))
			ON CONFLICT(user_id, group_id) DO UPDATE SET last_read_id = excluded.last_read_id`, me.ID, c.group)
		event["group_id"] = c.group
	} else {
		_, err = s.db.Exec(`INSERT INTO conversation_reads(user_id, peer_id, last_read_id)
			VALUES (?1, ?2, COALESCE((SELECT MAX(id) FROM messages WHERE group_id IS NULL AND sender_id = ?2 AND recipient_id = ?1), 0))
			ON CONFLICT(user_id, peer_id) DO UPDATE SET last_read_id = excluded.last_read_id`, me.ID, c.peer)
		event["peer_id"] = c.peer
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	unread, err := s.unreadMessages(me.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	event["unread"] = unread
	s.hub.sendTo(me.ID, event)
	writeJSON(w, http.StatusOK, map[string]int{"unread": unread})
}

// chatMessage loads message {mid} if it belongs to the chat.
func (s *server) chatMessage(w http.ResponseWriter, r *http.Request, c chat) (Message, bool) {
	mid, ok := pathID(w, r, "mid")
	if !ok {
		return Message{}, false
	}
	m, err := s.loadMessage(mid)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !c.contains(m)) {
		writeError(w, http.StatusNotFound, "message not found")
		return m, false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return m, false
	}
	return m, true
}

// updateChat edits the text of one of the caller's messages; everyone in the
// chat gets a message.updated event.
func (s *server) updateChat(w http.ResponseWriter, r *http.Request, me *User, c chat) {
	var in struct {
		Body string `json:"body"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	in.Body = strings.TrimSpace(in.Body)
	m, ok := s.chatMessage(w, r, c)
	if !ok {
		return
	}
	if m.SenderID != me.ID {
		writeError(w, http.StatusForbidden, "only the sender can edit a message")
		return
	}
	if !s.validMessage(w, in.Body, m.Attachments) {
		return
	}
	if _, err := s.db.Exec(`UPDATE messages SET body = ?, edited_at = ? WHERE id = ?`, in.Body, nowString(), m.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	m, err := s.loadMessage(m.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.hub.sendToMany(s.chatAudience(m), map[string]any{"type": "message.updated", "message": m})
	writeJSON(w, http.StatusOK, m)
}

// deleteChat removes a message for everyone in the chat: the sender's own, or,
// in a group, any message when the caller is the admin. Badges are refreshed
// by the message.deleted event.
func (s *server) deleteChat(w http.ResponseWriter, r *http.Request, me *User, c chat) {
	m, ok := s.chatMessage(w, r, c)
	if !ok {
		return
	}
	if m.SenderID != me.ID {
		role := ""
		if c.isGroup() {
			role, _ = s.groupRole(c.group, me.ID)
		}
		if role != "admin" {
			writeError(w, http.StatusForbidden, "only the sender can delete a message")
			return
		}
	}
	if _, err := s.db.Exec(`DELETE FROM messages WHERE id = ?`, m.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// SQLite hands the highest id out again after a delete, so a read marker
	// sitting at or past this message is pulled back to what remains; a new
	// message reusing the id then still counts as unread.
	var err error
	if c.isGroup() {
		_, err = s.db.Exec(`UPDATE group_reads SET last_read_id = (SELECT COALESCE(MAX(id), 0) FROM messages WHERE group_id = ?1)
			WHERE group_id = ?1 AND last_read_id >= ?2`, c.group, m.ID)
	} else {
		_, err = s.db.Exec(`UPDATE conversation_reads SET last_read_id = (SELECT COALESCE(MAX(id), 0) FROM messages WHERE group_id IS NULL AND sender_id = ?1 AND recipient_id = ?2)
			WHERE user_id = ?2 AND peer_id = ?1 AND last_read_id >= ?3`, m.SenderID, *m.RecipientID, m.ID)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, uid := range s.chatAudience(m) {
		unread, _ := s.unreadMessages(uid)
		s.hub.sendTo(uid, map[string]any{"type": "message.deleted", "id": m.ID, "sender_id": m.SenderID, "recipient_id": m.RecipientID, "group_id": m.GroupID, "unread": unread})
	}
	w.WriteHeader(http.StatusNoContent)
}

// listConversations returns the Messages tab: every direct chat with messages
// and every group the user belongs to, most recent activity first.
func (s *server) listConversations(w http.ResponseWriter, r *http.Request, me *User) {
	rows, err := s.db.Query(`
		SELECT `+messageCols+`, `+userCols+`,
			(SELECT COUNT(*) FROM messages x WHERE x.group_id IS NULL AND x.sender_id = u.id AND x.recipient_id = ?1
				AND x.id > COALESCE((SELECT last_read_id FROM conversation_reads r WHERE r.user_id = ?1 AND r.peer_id = u.id), 0))
		FROM messages m
		JOIN users u ON u.id = CASE WHEN m.sender_id = ?1 THEN m.recipient_id ELSE m.sender_id END
		WHERE m.id IN (
			SELECT MAX(id) FROM messages WHERE group_id IS NULL AND (sender_id = ?1 OR recipient_id = ?1)
			GROUP BY CASE WHEN sender_id = ?1 THEN recipient_id ELSE sender_id END)`, me.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := []Conversation{}
	for rows.Next() {
		var c Conversation
		var m Message
		var u User
		var atts string
		if err := rows.Scan(&m.ID, &m.SenderID, &m.RecipientID, &m.GroupID, &m.Body, &atts, &m.CreatedAt, &m.EditedAt,
			&u.ID, &u.Username, &u.DisplayName, &u.CreatedAt, &u.AvatarURL, &u.Pronouns, &u.AvatarCrop, &c.Unread); err != nil {
			rows.Close()
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		m.Attachments = decodeAttachments(atts)
		c.User, c.LastMessage = &u, &m
		out = append(out, c)
	}
	rows.Close()
	groups, err := s.queryGroups(me.ID, `WHERE g.id IN (SELECT group_id FROM group_members WHERE user_id = ?1)`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for i := range groups {
		g := &groups[i]
		c := Conversation{Group: g, Unread: s.groupUnread(me.ID, g.ID)}
		m, err := scanMessage(s.db.QueryRow(`SELECT `+messageCols+`, `+userCols+` FROM messages m JOIN users u ON u.id = m.sender_id
			WHERE m.group_id = ? ORDER BY m.id DESC LIMIT 1`, g.ID))
		if err == nil {
			c.LastMessage = &m
		} else if !errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i].LastMessage, out[j].LastMessage
		switch {
		case a != nil && b != nil:
			return a.CreatedAt > b.CreatedAt
		case a != nil || b != nil:
			return a != nil // chats with messages first
		default:
			return out[i].Group.Name < out[j].Group.Name
		}
	})
	writeJSON(w, http.StatusOK, out)
}
