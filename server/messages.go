package main

import (
	"net/http"
	"strings"
)

type Message struct {
	ID          int64  `json:"id"`
	SenderID    int64  `json:"sender_id"`
	RecipientID int64  `json:"recipient_id"`
	Body        string `json:"body"`
	CreatedAt   string `json:"created_at"`
}

type Conversation struct {
	User        User    `json:"user"`
	LastMessage Message `json:"last_message"`
}

// listConversations returns one entry per chat partner, newest activity first.
func (s *server) listConversations(w http.ResponseWriter, r *http.Request, me *User) {
	rows, err := s.db.Query(`
		SELECT m.id, m.sender_id, m.recipient_id, m.body, m.created_at, `+userCols+`
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
		if err := rows.Scan(&c.LastMessage.ID, &c.LastMessage.SenderID, &c.LastMessage.RecipientID, &c.LastMessage.Body, &c.LastMessage.CreatedAt,
			&c.User.ID, &c.User.Username, &c.User.DisplayName, &c.User.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
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
	rows, err := s.db.Query(`SELECT id, sender_id, recipient_id, body, created_at FROM (
			SELECT * FROM messages
			WHERE ((sender_id = ?1 AND recipient_id = ?2) OR (sender_id = ?2 AND recipient_id = ?1)) AND id < ?3
			ORDER BY id DESC LIMIT ?4)
		ORDER BY id ASC`, me.ID, other, before, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []Message{}
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.SenderID, &m.RecipientID, &m.Body, &m.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, m)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) sendMessage(w http.ResponseWriter, r *http.Request, me *User) {
	other, ok := pathID(w, r, "id")
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
	if !clampLen(in.Body, 4000) {
		writeError(w, http.StatusBadRequest, "body must be 1-4000 characters")
		return
	}
	if friends, err := s.areFriends(me.ID, other); err != nil || !friends {
		writeError(w, http.StatusForbidden, "you can only message friends")
		return
	}
	m := Message{SenderID: me.ID, RecipientID: other, Body: in.Body, CreatedAt: nowString()}
	res, err := s.db.Exec(`INSERT INTO messages(sender_id, recipient_id, body, created_at) VALUES (?, ?, ?, ?)`,
		m.SenderID, m.RecipientID, m.Body, m.CreatedAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	m.ID, _ = res.LastInsertId()
	writeJSON(w, http.StatusCreated, m)
}
