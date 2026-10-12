package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// presenceGrace is how long a user stays "online" after their last
// connection drops, so reconnects do not flicker.
const presenceGrace = 5 * time.Second

// hub tracks every live WebSocket connection per user and fans events out.
type hub struct {
	mu       sync.RWMutex
	users    map[int64]map[*client]struct{}
	leaving  map[int64]*time.Timer // users inside the presence grace period
	decorate func(v any) any       // fills presence into outgoing events (set by the server)
}

type client struct {
	user *User
	ws   *websocket.Conn
	send chan []byte
	done chan struct{}
}

func newHub() *hub {
	return &hub{users: map[int64]map[*client]struct{}{}, leaving: map[int64]*time.Timer{}}
}

func (h *hub) add(c *client) {
	h.mu.Lock()
	set := h.users[c.user.ID]
	if set == nil {
		set = map[*client]struct{}{}
		h.users[c.user.ID] = set
	}
	set[c] = struct{}{}
	wasPresent := len(set) > 1
	if t := h.leaving[c.user.ID]; t != nil { // back before the grace ran out
		t.Stop()
		delete(h.leaving, c.user.ID)
		wasPresent = true
	}
	h.mu.Unlock()
	if !wasPresent {
		h.sendAll(map[string]any{"type": "presence", "user_id": c.user.ID, "online": true})
	}
}

func (h *hub) remove(c *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	set := h.users[c.user.ID]
	if set == nil {
		return
	}
	delete(set, c)
	if len(set) > 0 {
		return
	}
	delete(h.users, c.user.ID)
	id := c.user.ID
	h.leaving[id] = time.AfterFunc(presenceGrace, func() {
		h.mu.Lock()
		_, back := h.users[id]
		delete(h.leaving, id)
		h.mu.Unlock()
		if !back {
			h.sendAll(map[string]any{"type": "presence", "user_id": id, "online": false})
		}
	})
}

// online is whether events can be delivered right now.
func (h *hub) online(userID int64) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.users[userID]) > 0
}

// present is what others see: connected, or gone for less than the grace period.
func (h *hub) present(userID int64) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.users[userID]) > 0 || h.leaving[userID] != nil
}

// onlineIDs lists every user currently shown as online.
func (h *hub) onlineIDs() []int64 {
	h.mu.RLock()
	defer h.mu.RUnlock()
	ids := make([]int64, 0, len(h.users)+len(h.leaving))
	for id := range h.users {
		ids = append(ids, id)
	}
	for id := range h.leaving {
		ids = append(ids, id)
	}
	return ids
}

func (h *hub) marshal(v any) ([]byte, error) {
	if h.decorate != nil {
		v = h.decorate(v)
	}
	return json.Marshal(v)
}

// sendAll pushes an event to every connection.
func (h *hub) sendAll(v any) {
	data, err := h.marshal(v)
	if err != nil {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, set := range h.users {
		for c := range set {
			select {
			case c.send <- data:
			default:
			}
		}
	}
}

// sendTo pushes a JSON event to every connection of a user. Slow clients are
// skipped rather than blocking the caller. Returns true if anyone was online.
func (h *hub) sendTo(userID int64, v any) bool {
	data, err := h.marshal(v)
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

// sendToMany fans an event out to several users.
func (h *hub) sendToMany(userIDs []int64, v any) {
	data, err := h.marshal(v)
	if err != nil {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, id := range userIDs {
		for c := range h.users[id] {
			select {
			case c.send <- data:
			default:
			}
		}
	}
}

// writeLoop is the connection's only writer (gorilla allows one at a time).
func (c *client) writeLoop() {
	ticker := time.NewTicker(wsPingEvery)
	defer ticker.Stop()
	for {
		select {
		case msg := <-c.send:
			_ = c.ws.SetWriteDeadline(time.Now().Add(wsWriteWait))
			if err := c.ws.WriteMessage(websocket.TextMessage, msg); err != nil {
				c.ws.Close()
				return
			}
		case <-ticker.C:
			if err := c.ws.WriteControl(websocket.PingMessage, nil, time.Now().Add(wsWriteWait)); err != nil {
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
	ws, err := s.wsUpgrade(w, r)
	if err != nil {
		return
	}
	c := &client{user: me, ws: ws, send: make(chan []byte, 64), done: make(chan struct{})}
	s.hub.add(c)
	go c.writeLoop()
	defer func() {
		s.hub.remove(c)
		close(c.done)
		_ = ws.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
		ws.Close()
	}()
	unread, _ := s.unreadMessages(me.ID)
	s.hub.sendTo(me.ID, map[string]any{"type": "hello", "user": me, "unread": unread, "online": s.hub.onlineIDs()})
	burst, window := 0, time.Now()
	for {
		_ = ws.SetReadDeadline(time.Now().Add(wsPongWait))
		_, data, err := ws.ReadMessage()
		if err != nil {
			return
		}
		if t := time.Now(); t.Sub(window) > 10*time.Second {
			burst, window = 0, t
		}
		if burst++; burst > wsMaxBurst {
			return // flooding
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
