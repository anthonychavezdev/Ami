package main

import (
	"log"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Calls are rooms: a one-to-one call between friends ("u:<lowID>:<highID>") or
// a group call any member may join ("g:<groupID>"). Participants send media to
// each other directly (WebRTC, full mesh), so a group call is capped at
// maxCallParticipants. The server only tracks who is in each room, rings the
// people who may join, and relays signaling between participants.
//
// Client -> server:
//
//	{"type":"call.join", "user_id":U, "video":bool}   call a friend, or pick up their call
//	{"type":"call.join", "group_id":G, "video":bool}  start or join a group call
//	{"type":"call.join", "room":"g:3"}                same, by the room key from call.ring
//	{"type":"call.leave", "room":key}
//	{"type":"call.decline", "room":key, "reason":"busy"}   (reason optional)
//	{"type":"call.offer"|"call.answer"|"call.ice"|"call.streams", "room":key, "to":U, ...}
//	    relayed to U with "from" (and "from_user" on offers) filled in
//
// Server -> client:
//
//	call.ring        {room, video, from_user, group?}           a call you can join was started
//	call.joined      {room, video, group?, participants:[users]} you are in (sent to the joining device only); offer to those listed
//	call.peer_joined {room, user}  /  call.peer_left {room, user_id}   (a user's other devices see themselves join: stop ringing)
//	call.declined    {room, user_id, reason}                    (to the participants)
//	call.ended       {room}                                     the room is empty again
//	call.unavailable {room, user_id}                            one-to-one: the friend is offline
//	error            {error}
//
// Everyone who could join hears about joins, leaves and the end of a call: the
// group's members, or both parties of a one-to-one call.
//
// A seat belongs to the connection that took it. When that connection drops,
// the seat is freed after the presence grace period unless the same user has
// re-asserted it with another call.join (an app reconnecting does that); an
// app that was restarted never does, so it does not linger as a ghost.
const maxCallParticipants = 8

type callMember struct {
	user *User
	conn *client // the device in the call
}

type callRoom struct {
	key     string
	groupID int64    // group call, or 0
	peers   [2]int64 // one-to-one call: both users, lowest id first
	video   bool     // placed as a video call
	members map[int64]*callMember
}

type callRooms struct {
	mu    sync.Mutex
	rooms map[string]*callRoom
}

func newCallRooms() *callRooms { return &callRooms{rooms: map[string]*callRoom{}} }

func groupRoomKey(groupID int64) string { return "g:" + strconv.FormatInt(groupID, 10) }

func directRoomKey(a, b int64) string {
	if a > b {
		a, b = b, a
	}
	return "u:" + strconv.FormatInt(a, 10) + ":" + strconv.FormatInt(b, 10)
}

// parseRoomKey reads a room key back into a group id or a pair of users.
func parseRoomKey(key string) (groupID int64, peers [2]int64, ok bool) {
	parts := strings.Split(key, ":")
	switch {
	case len(parts) == 2 && parts[0] == "g":
		id, err := strconv.ParseInt(parts[1], 10, 64)
		return id, peers, err == nil && id > 0
	case len(parts) == 3 && parts[0] == "u":
		a, err1 := strconv.ParseInt(parts[1], 10, 64)
		b, err2 := strconv.ParseInt(parts[2], 10, 64)
		return 0, [2]int64{a, b}, err1 == nil && err2 == nil && a > 0 && b > a
	}
	return 0, peers, false
}

// participants lists who is in a room right now, lowest id first.
func (c *callRooms) participants(key string) []int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	ids := []int64{}
	if r := c.rooms[key]; r != nil {
		for id := range r.members {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// callRoomFor resolves a call.join payload to a room, checking the caller may be in it.
func (s *server) callRoomFor(me *User, msg map[string]any) (key string, groupID int64, peers [2]int64, reason string) {
	if k, _ := msg["room"].(string); k != "" {
		g, p, ok := parseRoomKey(k)
		if !ok {
			return "", 0, peers, "unknown call"
		}
		groupID, peers = g, p
	} else if g, _ := msg["group_id"].(float64); g > 0 {
		groupID = int64(g)
	} else if u, _ := msg["user_id"].(float64); u > 0 {
		peers = [2]int64{me.ID, int64(u)}
	} else {
		return "", 0, peers, "say who to call"
	}
	if groupID != 0 {
		if role, err := s.groupRole(groupID, me.ID); err != nil || role == "" {
			return "", 0, peers, "only members can join a group call"
		}
		return groupRoomKey(groupID), groupID, peers, ""
	}
	other := peers[0]
	if other == me.ID {
		other = peers[1]
	}
	if other == me.ID || (peers[0] != me.ID && peers[1] != me.ID) {
		return "", 0, peers, "you cannot call yourself"
	}
	if ok, err := s.areFriends(me.ID, other); err != nil || !ok {
		return "", 0, peers, "you can only call friends"
	}
	if peers[0] > peers[1] {
		peers[0], peers[1] = peers[1], peers[0]
	}
	return directRoomKey(peers[0], peers[1]), 0, peers, ""
}

// callAudience is everyone told about a room: the group's members, or both
// parties of a one-to-one call.
func (s *server) callAudience(r *callRoom) []int64 {
	if r.groupID == 0 {
		return []int64{r.peers[0], r.peers[1]}
	}
	return s.groupMemberIDs(r.groupID)
}

// groupSummary is what call and message events say about a group.
func (s *server) groupSummary(id int64) map[string]any {
	var name string
	_ = s.db.QueryRow(`SELECT name FROM groups WHERE id = ?`, id).Scan(&name)
	return map[string]any{"id": id, "name": name}
}

func without(ids []int64, id int64) []int64 {
	out := make([]int64, 0, len(ids))
	for _, x := range ids {
		if x != id {
			out = append(out, x)
		}
	}
	return out
}

func (s *server) handleCallMessage(c *client, typ string, msg map[string]any) {
	me := c.user
	fail := func(text string) {
		log.Printf("call: %s from user %d refused: %s", typ, me.ID, text)
		s.hub.sendToClient(c, map[string]any{"type": "error", "error": text})
	}
	key, _ := msg["room"].(string)
	switch typ {
	case "call.join", "call.leave", "call.decline":
		log.Printf("call: %s from user %d room %q", typ, me.ID, key)
	}
	switch typ {
	case "call.join":
		if reason := s.joinCall(c, msg); reason != "" {
			fail(reason)
		}
	case "call.leave":
		s.leaveCall(me.ID, key)
	case "call.decline":
		reason, _ := msg["reason"].(string)
		s.declineCall(me, key, reason)
	case "call.offer", "call.answer", "call.ice", "call.streams":
		toF, _ := msg["to"].(float64)
		to := int64(toF)
		s.calls.mu.Lock()
		r := s.calls.rooms[key]
		ok := r != nil && r.members[me.ID] != nil && r.members[to] != nil && to != me.ID
		s.calls.mu.Unlock()
		if !ok {
			fail("not in that call")
			return
		}
		delete(msg, "to")
		msg["from"] = me.ID
		if typ == "call.offer" {
			msg["from_user"] = me
		}
		s.hub.sendTo(to, msg)
	default:
		fail("unknown call message")
	}
}

// joinCall puts the caller in a room, creating it (and ringing everyone who
// may join) when it is new. Returns a reason when the caller may not join.
func (s *server) joinCall(c *client, msg map[string]any) string {
	me := c.user
	key, groupID, peers, reason := s.callRoomFor(me, msg)
	if reason != "" {
		return reason
	}
	s.leaveOtherCalls(me.ID, key) // one call at a time
	video, _ := msg["video"].(bool)
	s.calls.mu.Lock()
	r := s.calls.rooms[key]
	created := r == nil
	if created {
		r = &callRoom{key: key, groupID: groupID, peers: peers, video: video, members: map[int64]*callMember{}}
		s.calls.rooms[key] = r
	}
	if r.members[me.ID] == nil && len(r.members) >= maxCallParticipants {
		s.calls.mu.Unlock()
		return "the call is full"
	}
	others := make([]*User, 0, len(r.members))
	for id, m := range r.members {
		if id != me.ID {
			others = append(others, m.user)
		}
	}
	sort.Slice(others, func(i, j int) bool { return others[i].ID < others[j].ID })
	reassert := r.members[me.ID] != nil
	r.members[me.ID] = &callMember{user: me, conn: c}
	video = r.video
	s.calls.mu.Unlock()

	var group map[string]any
	if groupID != 0 {
		group = s.groupSummary(groupID)
	}
	joined := map[string]any{"type": "call.joined", "room": key, "video": video, "participants": others}
	if group != nil {
		joined["group"] = group
	}
	s.hub.sendToClient(c, joined)
	if reassert {
		return "" // a reconnecting device: nothing changed for the others
	}
	audience := s.callAudience(r)
	if !created {
		s.hub.sendToMany(audience, map[string]any{"type": "call.peer_joined", "room": key, "user": me})
		return ""
	}
	audience = without(audience, me.ID)
	ring := map[string]any{"type": "call.ring", "room": key, "video": video, "from_user": me}
	if group != nil {
		ring["group"] = group
	}
	reached := false
	for _, uid := range audience {
		if s.hub.sendTo(uid, ring) {
			reached = true
		}
	}
	if groupID == 0 && !reached {
		// Nobody to pick up: tell the caller and forget the room.
		s.hub.sendTo(me.ID, map[string]any{"type": "call.unavailable", "room": key, "user_id": audience[0]})
		s.leaveCall(me.ID, key)
	}
	return ""
}

// leaveCall takes a user out of a room; when the room empties everyone hears
// call.ended, which stops ringing phones and clears "call in progress" banners.
func (s *server) leaveCall(userID int64, key string) {
	s.calls.mu.Lock()
	r := s.calls.rooms[key]
	if r == nil || r.members[userID] == nil {
		s.calls.mu.Unlock()
		return
	}
	delete(r.members, userID)
	empty := len(r.members) == 0
	if empty {
		delete(s.calls.rooms, key)
	}
	s.calls.mu.Unlock()
	audience := s.callAudience(r)
	s.hub.sendToMany(audience, map[string]any{"type": "call.peer_left", "room": key, "user_id": userID})
	if empty {
		s.hub.sendToMany(audience, map[string]any{"type": "call.ended", "room": key})
	}
}

// leaveOtherCalls drops the user from every room except `keep`.
func (s *server) leaveOtherCalls(userID int64, keep string) {
	s.calls.mu.Lock()
	var keys []string
	for k, r := range s.calls.rooms {
		if k != keep && r.members[userID] != nil {
			keys = append(keys, k)
		}
	}
	s.calls.mu.Unlock()
	for _, k := range keys {
		s.leaveCall(userID, k)
	}
}

// leaveCallsOf frees the seats a dropped connection still holds (the user did
// not re-assert them from another connection).
func (s *server) leaveCallsOf(c *client) {
	s.calls.mu.Lock()
	var keys []string
	for k, r := range s.calls.rooms {
		if m := r.members[c.user.ID]; m != nil && m.conn == c {
			keys = append(keys, k)
		}
	}
	s.calls.mu.Unlock()
	for _, k := range keys {
		s.leaveCall(c.user.ID, k)
	}
}

// endGroupCall closes a group's room (the group is being deleted).
func (s *server) endGroupCall(groupID int64) {
	key := groupRoomKey(groupID)
	for _, id := range s.calls.participants(key) {
		s.leaveCall(id, key)
	}
}

// declineCall tells the participants that someone who was rung is not coming.
func (s *server) declineCall(me *User, key, reason string) {
	s.calls.mu.Lock()
	r := s.calls.rooms[key]
	var members []int64
	if r != nil && r.members[me.ID] == nil {
		for id := range r.members {
			members = append(members, id)
		}
	}
	s.calls.mu.Unlock()
	if r == nil || members == nil {
		return
	}
	allowed := false
	for _, id := range s.callAudience(r) {
		if id == me.ID {
			allowed = true
		}
	}
	if !allowed {
		return
	}
	s.hub.sendToMany(members, map[string]any{"type": "call.declined", "room": key, "user_id": me.ID, "reason": reason})
}
