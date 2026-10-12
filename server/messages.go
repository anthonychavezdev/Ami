package main

import (
	"encoding/json"
	"net/http"
	"strings"
)

type Message struct {
	ID          int64        `json:"id"`
	SenderID    int64        `json:"sender_id"`
	RecipientID int64        `json:"recipient_id"`
	Body        string       `json:"body"`
	Attachments []Attachment `json:"attachments"`
	CreatedAt   string       `json:"created_at"`
}

type Conversation struct {
	User        User    `json:"user"`
	LastMessage Message `json:"last_message"`
}

const messageCols = "m.id, m.sender_id, m.recipient_id, m.body, m.attachments, m.created_at"

// decodeAttachments turns the stored JSON column into a (never nil) slice.
func decodeAttachments(raw string) []Attachment {
	out := []Attachment{}
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}

// listConversations returns one entry per chat partner, newest activity first.
func (s *server) listConversations(w http.ResponseWriter, r *http.Request, me *User) {
	rows, err := s.db.Query(`
		SELECT `+messageCols+`, `+userCols+`
		FROM messages m
		JOIN users u ON u.id = CASE WHEN m.sender_id = ?1 THEN m.recipient_id ELSE m.sender_id END
		WHERE m.id IN (
			SELECT MAX(id) FROM messages WHERE sender_id = ?1 OR recipient_id = ?1
			GROUP BY CASE WHEN sender_id = ?1 THEN recipient_id ELSE sender_id END)
		ORDER BY m.id DESC`, me.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []Conversation{}
	for rows.Next() {
		var c Conversation
		var atts string
		if err := rows.Scan(&c.LastMessage.ID, &c.LastMessage.SenderID, &c.LastMessage.RecipientID, &c.LastMessage.Body, &atts, &c.LastMessage.CreatedAt,
			&c.User.ID, &c.User.Username, &c.User.DisplayName, &c.User.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		c.LastMessage.Attachments = decodeAttachments(atts)
		out = append(out, c)
	}
	writeJSON(w, http.StatusOK, out)
}

// listMessages returns the newest messages with one user, oldest first within the page.
func (s *server) listMessages(w http.ResponseWriter, r *http.Request, me *User) {
	other, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	before, limit := pagination(r)
	if limit == 30 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT `+messageCols+` FROM (
			SELECT * FROM messages
			WHERE ((sender_id = ?1 AND recipient_id = ?2) OR (sender_id = ?2 AND recipient_id = ?1)) AND id < ?3
			ORDER BY id DESC LIMIT ?4) m
		ORDER BY m.id ASC`, me.ID, other, before, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []Message{}
	for rows.Next() {
		var m Message
		var atts string
		if err := rows.Scan(&m.ID, &m.SenderID, &m.RecipientID, &m.Body, &atts, &m.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		m.Attachments = decodeAttachments(atts)
		out = append(out, m)
	}
	writeJSON(w, http.StatusOK, out)
}

// sendMessage stores a message (text, pictures/videos uploaded via /api/media,
// or both) and pushes it to both participants over the WebSocket.
func (s *server) sendMessage(w http.ResponseWriter, r *http.Request, me *User) {
	other, ok := pathID(w, r, "id")
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
		writeError(w, http.StatusBadRequest, "a message needs text or an attachment")
		return
	}
	if len(in.Body) > 4000 {
		writeError(w, http.StatusBadRequest, "body must be at most 4000 characters")
		return
	}
	if in.Attachments == nil {
		in.Attachments = []Attachment{}
	}
	if err := s.validateAttachments(in.Attachments); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if friends, err := s.areFriends(me.ID, other); err != nil || !friends {
		writeError(w, http.StatusForbidden, "you can only message friends")
		return
	}
	attachments, _ := json.Marshal(in.Attachments)
	m := Message{SenderID: me.ID, RecipientID: other, Body: in.Body, Attachments: in.Attachments, CreatedAt: nowString()}
	res, err := s.db.Exec(`INSERT INTO messages(sender_id, recipient_id, body, attachments, created_at) VALUES (?, ?, ?, ?, ?)`,
		m.SenderID, m.RecipientID, m.Body, string(attachments), m.CreatedAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	m.ID, _ = res.LastInsertId()
	event := map[string]any{"type": "message", "message": m, "sender": me}
	s.hub.sendTo(other, event)
	s.hub.sendTo(me.ID, event) // keep the sender's other devices in sync
	writeJSON(w, http.StatusCreated, m)
}
