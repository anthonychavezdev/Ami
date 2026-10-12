package main

import (
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

// WebSocket transport on top of gorilla/websocket. Connections authenticate
// with the session token (?token= or Authorization header), so any Origin is
// accepted: a page cannot open a socket without knowing the token.
const (
	wsMaxMessage = 64 << 10
	wsPingEvery  = 30 * time.Second
	wsPongWait   = 90 * time.Second // peers that stop answering pings are dropped
	wsWriteWait  = 10 * time.Second
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     func(*http.Request) bool { return true },
}

// wsUpgrade performs the opening handshake. On failure it has already written
// an HTTP error response.
func wsUpgrade(w http.ResponseWriter, r *http.Request) (*websocket.Conn, error) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return nil, err
	}
	ws.SetReadLimit(wsMaxMessage)
	_ = ws.SetReadDeadline(time.Now().Add(wsPongWait))
	ws.SetPongHandler(func(string) error { return ws.SetReadDeadline(time.Now().Add(wsPongWait)) })
	return ws, nil
}
