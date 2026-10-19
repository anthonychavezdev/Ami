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
// connection drops, so reconnects do not flicker. A variable so tests can
// shorten it.
var presenceGrace = 5 * time.Second

// hub tracks every live WebSocket connection per user and fans events out.
type hub struct {
	mu       sync.RWMutex
	users    map[string]map[*client]struct{}
	leaving  map[string]*time.Timer // users inside the presence grace period
	hidden   map[string]bool        // users who chose to appear offline
	decorate func(v any) any        // fills presence into outgoing events (set by the server)
}

type client struct {
	user *User
	ws   *websocket.Conn
	send chan []byte
	done chan struct{}
}

func newHub() *hub {
	return &hub{users: map[string]map[*client]struct{}{}, leaving: map[string]*time.Timer{}, hidden: map[string]bool{}}
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
	hidden := h.hidden[c.user.ID]
	h.mu.Unlock()
	if !wasPresent && !hidden {
		h.sendAll(presenceEvent{Type: "presence", UserID: c.user.ID, Online: true})
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
		hidden := h.hidden[id]
		delete(h.leaving, id)
		h.mu.Unlock()
		if !back && !hidden {
			h.sendAll(presenceEvent{Type: "presence", UserID: id, Online: false})
		}
	})
}

// closeAll drops every connection (shutdown); each handleWS then runs its cleanup.
func (h *hub) closeAll() {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, set := range h.users {
		for c := range set {
			c.ws.Close()
		}
	}
}

// online is whether events can be delivered right now.
func (h *hub) online(userID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.users[userID]) > 0
}

// present is what others see: connected (or gone for less than the grace
// period) and not appearing offline on purpose.
func (h *hub) present(userID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return !h.hidden[userID] && (len(h.users[userID]) > 0 || h.leaving[userID] != nil)
}

// setHidden records whether a user appears offline; if they are connected the
// change is announced like a disconnect or a reconnect.
func (h *hub) setHidden(userID string, hidden bool) {
	h.mu.Lock()
	was := h.hidden[userID]
	if hidden {
		h.hidden[userID] = true
	} else {
		delete(h.hidden, userID)
	}
	connected := len(h.users[userID]) > 0 || h.leaving[userID] != nil
	h.mu.Unlock()
	if was != hidden && connected {
		h.sendAll(presenceEvent{Type: "presence", UserID: userID, Online: !hidden})
	}
}

// onlineIDs lists every user currently shown as online.
func (h *hub) onlineIDs() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	ids := make([]string, 0, len(h.users)+len(h.leaving))
	for id := range h.users {
		if !h.hidden[id] {
			ids = append(ids, id)
		}
	}
	for id := range h.leaving {
		if !h.hidden[id] {
			ids = append(ids, id)
		}
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
	h.mu.RLock()
	ids := make([]string, 0, len(h.users))
	for id := range h.users {
		ids = append(ids, id)
	}
	h.mu.RUnlock()
	h.deliver(ids, v)
}

// sendTo pushes an event to every connection of a user. Returns true if anyone was online.
func (h *hub) sendTo(userID string, v any) bool { return h.deliver([]string{userID}, v) > 0 }

// sendToMany fans an event out to several users.
func (h *hub) sendToMany(userIDs []string, v any) { h.deliver(userIDs, v) }

// sendToClient pushes an event to one connection only (the device that acted).
func (h *hub) sendToClient(c *client, v any) {
	if data, err := h.marshal(v); err == nil {
		if verbose {
			log.Printf("ws → user %s (one device): %s", c.user.ID, cut(data))
		}
		c.queue(data)
	}
}

// deliver queues an event on every connection of the users and reports how
// many connections got it. Slow clients are skipped rather than blocking.
func (h *hub) deliver(userIDs []string, v any) int {
	data, err := h.marshal(v)
	if err != nil {
		return 0
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	n := 0
	for _, id := range userIDs {
		for c := range h.users[id] {
			c.queue(data)
			n++
		}
	}
	if verbose {
		log.Printf("ws → users %v (connections: %d): %s", userIDs, n, cut(data))
	}
	return n
}

// queue hands a frame to the writer without waiting for it.
func (c *client) queue(data []byte) {
	select {
	case c.send <- data:
	default:
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
	su, err := s.userForToken(bearerToken(r))
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	me := su.User
	noteUser(r, me.ID)
	s.hub.setHidden(me.ID, s.invisible(me.ID)) // before add, so an invisible user is never announced
	ws, err := s.wsUpgrade(w, r)
	if err != nil {
		return
	}
	c := &client{user: me, ws: ws, send: make(chan []byte, 64), done: make(chan struct{})}
	s.hub.add(c)
	go c.writeLoop()
	defer func() {
		s.hub.remove(c)
		time.AfterFunc(presenceGrace, func() { s.leaveCallsOf(c) }) // hang up for a device that is gone for good
		close(c.done)
		_ = ws.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
		ws.Close()
	}()
	unread, _ := s.unreadMessages(me.ID)
	s.hub.sendTo(me.ID, helloEvent{Type: "hello", User: me, Unread: unread, Online: s.hub.onlineIDs()})
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

// handleWSMessage dispatches one client message (see clientMessage in events.go).
func (s *server) handleWSMessage(c *client, data []byte) {
	if verbose {
		log.Printf("ws ← user %s: %s", c.user.ID, cut(data))
	}
	var msg clientMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return
	}
	switch {
	case msg.Type == "ping":
		s.hub.sendTo(c.user.ID, pongEvent{Type: "pong"})
	case strings.HasPrefix(msg.Type, "call."):
		s.handleCallMessage(c, msg)
	default:
		log.Printf("ws: unknown message type %q from user %s", msg.Type, c.user.ID)
	}
}
