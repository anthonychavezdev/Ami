package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// hub tracks every live WebSocket connection per user and fans events out.
type hub struct {
	mu    sync.RWMutex
	users map[int64]map[*client]struct{}
}

type client struct {
	user *User
	ws   *wsConn
	send chan []byte
	done chan struct{}
}

func newHub() *hub { return &hub{users: map[int64]map[*client]struct{}{}} }

func (h *hub) add(c *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	set := h.users[c.user.ID]
	if set == nil {
		set = map[*client]struct{}{}
		h.users[c.user.ID] = set
	}
	set[c] = struct{}{}
}

func (h *hub) remove(c *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if set := h.users[c.user.ID]; set != nil {
		delete(set, c)
		if len(set) == 0 {
			delete(h.users, c.user.ID)
		}
	}
}

func (h *hub) online(userID int64) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.users[userID]) > 0
}

// sendTo pushes a JSON event to every connection of a user. Slow clients are
// skipped rather than blocking the caller. Returns true if anyone was online.
func (h *hub) sendTo(userID int64, v any) bool {
	data, err := json.Marshal(v)
	if err != nil {
		return false
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	set := h.users[userID]
	for c := range set {
		select {
		case c.send <- data:
		default:
		}
	}
	return len(set) > 0
}

func (c *client) writeLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case msg := <-c.send:
			if err := c.ws.WriteText(msg); err != nil {
				c.ws.Close()
				return
			}
		case <-ticker.C:
			if err := c.ws.Ping(); err != nil {
				c.ws.Close()
				return
			}
		case <-c.done:
			return
		}
	}
}

// handleWS upgrades GET /ws?token=... and serves the connection until it drops.
func (s *server) handleWS(w http.ResponseWriter, r *http.Request) {
	me, err := s.userForToken(bearerToken(r))
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	ws, err := wsUpgrade(w, r)
	if err != nil {
		return
	}
	c := &client{user: me, ws: ws, send: make(chan []byte, 64), done: make(chan struct{})}
	s.hub.add(c)
	go c.writeLoop()
	defer func() {
		s.hub.remove(c)
		close(c.done)
		ws.Close()
	}()
	s.hub.sendTo(me.ID, map[string]any{"type": "hello", "user": me})
	for {
		ws.SetReadDeadline(time.Now().Add(90 * time.Second))
		_, data, err := ws.ReadMessage()
		if err != nil {
			return
		}
		s.handleWSMessage(c, data)
	}
}

// Client -> server messages:
//
//	{"type":"ping"}
//	{"type":"call.offer",  "to":<userID>, "sdp":{...}, "video":bool}
//	{"type":"call.answer", "to":<userID>, "sdp":{...}}
//	{"type":"call.ice",    "to":<userID>, "candidate":{...}}
//	{"type":"call.streams","to":<userID>, "camera":"<streamID>", "screen":"<streamID>"}
//	{"type":"call.reject" | "call.hangup" | "call.busy", "to":<userID>}
//
// call.* messages are relayed verbatim to the recipient with "from" (and, for
// offers, "from_user") filled in by the server. Only friends can call each other.
func (s *server) handleWSMessage(c *client, data []byte) {
	var msg map[string]any
	if err := json.Unmarshal(data, &msg); err != nil {
		return
	}
	typ, _ := msg["type"].(string)
	switch {
	case typ == "ping":
		s.hub.sendTo(c.user.ID, map[string]any{"type": "pong"})
	case strings.HasPrefix(typ, "call."):
		toF, _ := msg["to"].(float64)
		to := int64(toF)
		if to <= 0 || to == c.user.ID {
			return
		}
		if ok, err := s.areFriends(c.user.ID, to); err != nil || !ok {
			s.hub.sendTo(c.user.ID, map[string]any{"type": "error", "error": "you can only call friends"})
			return
		}
		delete(msg, "to")
		msg["from"] = c.user.ID
		if typ == "call.offer" {
			msg["from_user"] = c.user
		}
		if !s.hub.sendTo(to, msg) {
			s.hub.sendTo(c.user.ID, map[string]any{"type": "call.unavailable", "from": to})
		}
	default:
		log.Printf("ws: unknown message type %q from user %d", typ, c.user.ID)
	}
}
