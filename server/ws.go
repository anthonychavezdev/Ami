package main

import (
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

// WebSocket transport on top of gorilla/websocket. Connections authenticate
// with the session token and must come from an allowed browser origin (see
// server.originAllowed); native apps send no Origin.
const (
	wsMaxMessage = 64 << 10
	wsMaxBurst   = 300 // inbound messages per 10 s before a flooding peer is dropped
	wsPingEvery  = 30 * time.Second
	wsPongWait   = 90 * time.Second // peers that stop answering pings are dropped
	wsWriteWait  = 10 * time.Second
)

// wsUpgrade performs the opening handshake. On failure it has already written
// an HTTP error response (403 for a disallowed origin).
func (s *server) wsUpgrade(w http.ResponseWriter, r *http.Request) (*websocket.Conn, error) {
	ws, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return nil, err
	}
	ws.SetReadLimit(wsMaxMessage)
	_ = ws.SetReadDeadline(time.Now().Add(wsPongWait))
	ws.SetPongHandler(func(string) error { return ws.SetReadDeadline(time.Now().Add(wsPongWait)) })
	return ws, nil
}
