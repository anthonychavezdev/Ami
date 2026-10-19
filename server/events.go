package main

import "encoding/json"

// Everything that travels over the WebSocket, in both directions. One struct
// per "type"; mobile/src/lib/events.ts mirrors them field for field, so a
// change here is a change there too.
//
// Server -> client events are pushed by the hub (hub.go) and built by the
// handlers; client -> server messages are decoded in handleWSMessage and
// handled in calls.go.

// groupRef is what events say about a group.
type groupRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// ---- server -> client

// helloEvent opens every connection: who you are, your unread total and who
// is online right now.
type helloEvent struct {
	Type   string  `json:"type"` // "hello"
	User   *User   `json:"user"`
	Unread int     `json:"unread"`
	Online []int64 `json:"online"`
}

type presenceEvent struct {
	Type   string `json:"type"` // "presence"
	UserID int64  `json:"user_id"`
	Online bool   `json:"online"`
}

type pongEvent struct {
	Type string `json:"type"` // "pong"
}

// errorEvent answers a client message that was refused.
type errorEvent struct {
	Type  string `json:"type"` // "error"
	Error string `json:"error"`
}

// messageEvent carries a new chat message. Recipients get their unread total
// with it; the sender's other devices get a plain copy.
type messageEvent struct {
	Type    string    `json:"type"` // "message"
	Message Message   `json:"message"`
	Sender  *User     `json:"sender"`
	Group   *groupRef `json:"group,omitempty"`
	Unread  *int      `json:"unread,omitempty"`
}

type messageUpdatedEvent struct {
	Type    string  `json:"type"` // "message.updated"
	Message Message `json:"message"`
}

// messageDeletedEvent names the chat the message was in, and the receiver's
// new unread total.
type messageDeletedEvent struct {
	Type        string `json:"type"` // "message.deleted"
	ID          int64  `json:"id"`
	SenderID    int64  `json:"sender_id"`
	RecipientID *int64 `json:"recipient_id"`
	GroupID     *int64 `json:"group_id"`
	Unread      int    `json:"unread"`
}

// messagesReadEvent tells a user's devices a chat was read (or stopped
// counting), with the new unread total.
type messagesReadEvent struct {
	Type    string `json:"type"` // "messages.read"
	PeerID  *int64 `json:"peer_id,omitempty"`
	GroupID *int64 `json:"group_id,omitempty"`
	Unread  int    `json:"unread"`
}

type groupAddedEvent struct {
	Type  string   `json:"type"` // "group.added"
	Group groupRef `json:"group"`
	By    *User    `json:"by"`
}

// friendChangedEvent is "friend.changed": the friendship with User now has
// Status (none, outgoing, incoming or friends) as the receiver sees it. Both
// people get one, the one who acted included, so every list and profile
// showing the other follows.
type friendChangedEvent struct {
	Type   string `json:"type"`
	User   *User  `json:"user"`
	Status string `json:"status"`
}

// postEvent is "post.created" or "post.updated", with the post as the
// receiver sees it (their own reaction filled in).
type postEvent struct {
	Type string `json:"type"`
	Post Post   `json:"post"`
}

type postDeletedEvent struct {
	Type    string `json:"type"` // "post.deleted"
	ID      int64  `json:"id"`
	GroupID *int64 `json:"group_id"`
}

// commentEvent is "comment.created" or "comment.updated".
type commentEvent struct {
	Type    string  `json:"type"`
	Comment Comment `json:"comment"`
}

type commentDeletedEvent struct {
	Type   string `json:"type"` // "comment.deleted"
	PostID int64  `json:"post_id"`
	ID     int64  `json:"id"`
}

// callRingEvent: a call you may join was started.
type callRingEvent struct {
	Type     string    `json:"type"` // "call.ring"
	Room     string    `json:"room"`
	Video    bool      `json:"video"`
	FromUser *User     `json:"from_user"`
	Group    *groupRef `json:"group,omitempty"`
}

// callJoinedEvent answers your call.join: you are in; offer to the participants listed.
type callJoinedEvent struct {
	Type         string    `json:"type"` // "call.joined"
	Room         string    `json:"room"`
	Video        bool      `json:"video"`
	Group        *groupRef `json:"group,omitempty"`
	Participants []*User   `json:"participants"`
}

type callPeerJoinedEvent struct {
	Type string `json:"type"` // "call.peer_joined"
	Room string `json:"room"`
	User *User  `json:"user"`
}

type callPeerLeftEvent struct {
	Type   string `json:"type"` // "call.peer_left"
	Room   string `json:"room"`
	UserID int64  `json:"user_id"`
}

type callDeclinedEvent struct {
	Type   string `json:"type"` // "call.declined"
	Room   string `json:"room"`
	UserID int64  `json:"user_id"`
	Reason string `json:"reason"`
}

// callRoomEvent is "call.ended" (the room is empty again) or
// "call.unavailable" (one-to-one: the friend is offline; UserID says who).
type callRoomEvent struct {
	Type   string `json:"type"`
	Room   string `json:"room"`
	UserID int64  `json:"user_id,omitempty"`
}

// callSignal is a relayed call.offer, call.answer, call.ice or call.streams:
// the sender's payload with "from" (and "from_user" on offers) filled in.
type callSignal struct {
	Type      string          `json:"type"`
	Room      string          `json:"room"`
	From      int64           `json:"from"`
	FromUser  *User           `json:"from_user,omitempty"`
	SDP       json.RawMessage `json:"sdp,omitempty"`
	Candidate json.RawMessage `json:"candidate,omitempty"`
	Camera    json.RawMessage `json:"camera,omitempty"`
	Screen    json.RawMessage `json:"screen,omitempty"`
}

// ---- client -> server

// clientMessage is any message a client sends: {"type":"ping"} or one of the
// call.* messages documented in calls.go. Fields not used by a type are empty.
type clientMessage struct {
	Type      string          `json:"type"`
	Room      string          `json:"room"`
	UserID    int64           `json:"user_id"`
	GroupID   int64           `json:"group_id"`
	Video     bool            `json:"video"`
	Reason    string          `json:"reason"`
	To        int64           `json:"to"`
	SDP       json.RawMessage `json:"sdp"`
	Candidate json.RawMessage `json:"candidate"`
	Camera    json.RawMessage `json:"camera"`
	Screen    json.RawMessage `json:"screen"`
}

// signal is the relayed form of a signaling message.
func (m clientMessage) signal(from *User) callSignal {
	out := callSignal{Type: m.Type, Room: m.Room, From: from.ID, SDP: m.SDP, Candidate: m.Candidate, Camera: m.Camera, Screen: m.Screen}
	if m.Type == "call.offer" {
		out.FromUser = from
	}
	return out
}
