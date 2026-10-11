package main

// Minimal RFC 6455 WebSocket server implementation using only the standard
// library: handshake, framing (with fragmentation), masking, ping/pong, close.

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	wsGUID       = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	wsMaxMessage = 1 << 20 // 1 MiB

	opContinuation = 0x0
	opText         = 0x1
	opBinary       = 0x2
	opClose        = 0x8
	opPing         = 0x9
	opPong         = 0xA
)

var errWSClosed = errors.New("websocket: closed")

type wsConn struct {
	conn   net.Conn
	rw     *bufio.ReadWriter
	wmu    sync.Mutex
	closed bool
}

func headerHas(h http.Header, key, token string) bool {
	for _, v := range h.Values(key) {
		for _, part := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(part), token) {
				return true
			}
		}
	}
	return false
}

// wsUpgrade performs the opening handshake and hijacks the connection.
// On failure it has already written an HTTP error response.
func wsUpgrade(w http.ResponseWriter, r *http.Request) (*wsConn, error) {
	key := r.Header.Get("Sec-WebSocket-Key")
	if r.Method != http.MethodGet || !headerHas(r.Header, "Connection", "upgrade") ||
		!strings.EqualFold(r.Header.Get("Upgrade"), "websocket") ||
		r.Header.Get("Sec-WebSocket-Version") != "13" || key == "" {
		http.Error(w, "bad websocket handshake", http.StatusBadRequest)
		return nil, errors.New("websocket: bad handshake")
	}
	conn, rw, err := http.NewResponseController(w).Hijack()
	if err != nil {
		http.Error(w, "hijack unsupported", http.StatusInternalServerError)
		return nil, err
	}
	sum := sha1.Sum([]byte(key + wsGUID))
	resp := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + base64.StdEncoding.EncodeToString(sum[:]) + "\r\n\r\n"
	conn.SetDeadline(time.Time{})
	if _, err := rw.WriteString(resp); err != nil {
		conn.Close()
		return nil, err
	}
	if err := rw.Flush(); err != nil {
		conn.Close()
		return nil, err
	}
	return &wsConn{conn: conn, rw: rw}, nil
}

// ReadMessage returns the next complete data message (text or binary),
// transparently answering pings and handling close frames.
func (c *wsConn) ReadMessage() (opcode int, payload []byte, err error) {
	var msg []byte
	msgOp := 0
	for {
		fin, op, data, err := c.readFrame()
		if err != nil {
			return 0, nil, err
		}
		switch op {
		case opClose:
			c.Close()
			return 0, nil, io.EOF
		case opPing:
			if err := c.writeFrame(opPong, data); err != nil {
				return 0, nil, err
			}
			continue
		case opPong:
			continue
		case opContinuation:
			if msgOp == 0 {
				return 0, nil, errors.New("websocket: unexpected continuation frame")
			}
		case opText, opBinary:
			if msgOp != 0 {
				return 0, nil, errors.New("websocket: new data frame inside fragmented message")
			}
			msgOp = op
		default:
			return 0, nil, errors.New("websocket: unknown opcode")
		}
		msg = append(msg, data...)
		if len(msg) > wsMaxMessage {
			return 0, nil, errors.New("websocket: message too large")
		}
		if fin {
			return msgOp, msg, nil
		}
	}
}

func (c *wsConn) readFrame() (fin bool, op int, payload []byte, err error) {
	var hdr [2]byte
	if _, err = io.ReadFull(c.rw, hdr[:]); err != nil {
		return
	}
	fin = hdr[0]&0x80 != 0
	if hdr[0]&0x70 != 0 {
		return false, 0, nil, errors.New("websocket: reserved bits set")
	}
	op = int(hdr[0] & 0x0f)
	masked := hdr[1]&0x80 != 0
	n := uint64(hdr[1] & 0x7f)
	switch n {
	case 126:
		var b [2]byte
		if _, err = io.ReadFull(c.rw, b[:]); err != nil {
			return
		}
		n = uint64(binary.BigEndian.Uint16(b[:]))
	case 127:
		var b [8]byte
		if _, err = io.ReadFull(c.rw, b[:]); err != nil {
			return
		}
		n = binary.BigEndian.Uint64(b[:])
	}
	if n > wsMaxMessage {
		return false, 0, nil, errors.New("websocket: frame too large")
	}
	if !masked {
		return false, 0, nil, errors.New("websocket: client frame not masked")
	}
	var mask [4]byte
	if _, err = io.ReadFull(c.rw, mask[:]); err != nil {
		return
	}
	payload = make([]byte, n)
	if _, err = io.ReadFull(c.rw, payload); err != nil {
		return
	}
	for i := range payload {
		payload[i] ^= mask[i&3]
	}
	return fin, op, payload, nil
}

func (c *wsConn) writeFrame(op int, payload []byte) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	if c.closed {
		return errWSClosed
	}
	return c.writeFrameLocked(op, payload)
}

func (c *wsConn) writeFrameLocked(op int, payload []byte) error {
	hdr := []byte{0x80 | byte(op)}
	switch n := len(payload); {
	case n < 126:
		hdr = append(hdr, byte(n))
	case n < 1<<16:
		hdr = append(hdr, 126)
		hdr = binary.BigEndian.AppendUint16(hdr, uint16(n))
	default:
		hdr = append(hdr, 127)
		hdr = binary.BigEndian.AppendUint64(hdr, uint64(n))
	}
	c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if _, err := c.rw.Write(hdr); err != nil {
		return err
	}
	if _, err := c.rw.Write(payload); err != nil {
		return err
	}
	return c.rw.Flush()
}

func (c *wsConn) WriteText(p []byte) error { return c.writeFrame(opText, p) }
func (c *wsConn) Ping() error              { return c.writeFrame(opPing, nil) }

// SetReadDeadline bounds the next read; combined with periodic pings it
// detects dead peers.
func (c *wsConn) SetReadDeadline(t time.Time) { c.conn.SetReadDeadline(t) }

// Close sends a close frame (code 1000) and closes the TCP connection.
func (c *wsConn) Close() error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	_ = c.writeFrameLocked(opClose, []byte{0x03, 0xE8})
	return c.conn.Close()
}
