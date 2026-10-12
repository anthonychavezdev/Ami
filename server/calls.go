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

// ---- room keys

func groupRoomKey(groupID int64) string { return "g:" + strconv.FormatInt(groupID, 10) }

func directRoomKey(a, b int64) string {
	return "u:" + strconv.FormatInt(min(a, b), 10) + ":" + strconv.FormatInt(max(a, b), 10)
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

// ---- messages from clients

func (s *server) handleCallMessage(c *client, typ string, msg map[string]any) {
	key, _ := msg["room"].(string)
	switch typ {
	case "call.join", "call.leave", "call.decline":
		log.Printf("call: %s from user %d room %q", typ, c.user.ID, key)
	}
	var err error
	switch typ {
	case "call.join":
		err = s.joinCall(c, msg)
	case "call.leave":
		s.leaveCall(c.user.ID, key)
	case "call.decline":
		reason, _ := msg["reason"].(string)
		s.declineCall(c.user, key, reason)
	case "call.offer", "call.answer", "call.ice", "call.streams":
		err = s.relayCall(c, key, typ, msg)
	default:
		err = fail(0, "unknown call message")
	}
	if err != nil {
		log.Printf("call: %s from user %d refused: %v", typ, c.user.ID, err)
		s.hub.sendToClient(c, map[string]any{"type": "error", "error": err.Error()})
	}
}

// relayCall forwards signaling to one other participant of the same room.
func (s *server) relayCall(c *client, key, typ string, msg map[string]any) error {
	toF, _ := msg["to"].(float64)
	to := int64(toF)
	s.calls.mu.Lock()
	r := s.calls.rooms[key]
	inRoom := r != nil && r.members[c.user.ID] != nil && r.members[to] != nil && to != c.user.ID
	s.calls.mu.Unlock()
	if !inRoom {
		return fail(0, "not in that call")
	}
	delete(msg, "to")
	msg["from"] = c.user.ID
	if typ == "call.offer" {
		msg["from_user"] = c.user
	}
	s.hub.sendTo(to, msg)
	return nil
}

// callRoomFor resolves a call.join payload to a room, checking the caller may be in it.
func (s *server) callRoomFor(me *User, msg map[string]any) (key string, groupID int64, peers [2]int64, err error) {
	if k, _ := msg["room"].(string); k != "" {
		g, p, ok := parseRoomKey(k)
		if !ok {
			return "", 0, peers, fail(0, "unknown call")
		}
		groupID, peers = g, p
	} else if g, _ := msg["group_id"].(float64); g > 0 {
		groupID = int64(g)
	} else if u, _ := msg["user_id"].(float64); u > 0 {
		peers = [2]int64{min(me.ID, int64(u)), max(me.ID, int64(u))}
	} else {
		return "", 0, peers, fail(0, "say who to call")
	}
	if groupID != 0 {
		if role, err := s.groupRole(groupID, me.ID); err != nil || role == "" {
			return "", 0, peers, fail(0, "only members can join a group call")
		}
		return groupRoomKey(groupID), groupID, peers, nil
	}
	other := peers[0] + peers[1] - me.ID
	if other == me.ID || (peers[0] != me.ID && peers[1] != me.ID) {
		return "", 0, peers, fail(0, "you cannot call yourself")
	}
	if friends, err := s.areFriends(me.ID, other); err != nil || !friends {
		return "", 0, peers, fail(0, "you can only call friends")
	}
	return directRoomKey(peers[0], peers[1]), 0, peers, nil
}

// callAudience is everyone told about a room: the group's members, or both
// parties of a one-to-one call.
func (s *server) callAudience(groupID int64, peers [2]int64) []int64 {
	if groupID == 0 {
		return []int64{peers[0], peers[1]}
	}
	return s.groupMemberIDs(groupID)
}

// withGroup adds the group to a call event when there is one.
func withGroup(event map[string]any, group map[string]any) map[string]any {
	if group != nil {
		event["group"] = group
	}
	return event
}

// seat is the outcome of taking a place in a room.
type seat struct {
	created  bool    // the room did not exist before
	reassert bool    // the user was already in it (a reconnecting device)
	video    bool    // how the call was placed
	others   []*User // who was there already, lowest id first
}

// takeSeat puts a connection's user in the room, creating it when new.
func (s *server) takeSeat(c *client, key string, groupID int64, peers [2]int64, video bool) (seat, error) {
	s.calls.mu.Lock()
	defer s.calls.mu.Unlock()
	r := s.calls.rooms[key]
	st := seat{created: r == nil, others: []*User{}}
	if st.created {
		r = &callRoom{key: key, groupID: groupID, peers: peers, video: video, members: map[int64]*callMember{}}
		s.calls.rooms[key] = r
	}
	st.reassert = r.members[c.user.ID] != nil
	if !st.reassert && len(r.members) >= maxCallParticipants {
		return st, fail(0, "the call is full")
	}
	for id, m := range r.members {
		if id != c.user.ID {
			st.others = append(st.others, m.user)
		}
	}
	sort.Slice(st.others, func(i, j int) bool { return st.others[i].ID < st.others[j].ID })
	r.members[c.user.ID] = &callMember{user: c.user, conn: c}
	st.video = r.video
	return st, nil
}

// joinCall puts the caller in a room and tells the others; a new room rings
// everyone who may join.
func (s *server) joinCall(c *client, msg map[string]any) error {
	me := c.user
	key, groupID, peers, err := s.callRoomFor(me, msg)
	if err != nil {
		return err
	}
	s.leaveOtherCalls(me.ID, key) // one call at a time
	video, _ := msg["video"].(bool)
	st, err := s.takeSeat(c, key, groupID, peers, video)
	if err != nil {
		return err
	}
	var group map[string]any
	if groupID != 0 {
		group = s.groupSummary(groupID)
	}
	s.hub.sendToClient(c, withGroup(map[string]any{"type": "call.joined", "room": key, "video": st.video, "participants": st.others}, group))
	if st.reassert {
		return nil // a reconnecting device: nothing changed for the others
	}
	audience := s.callAudience(groupID, peers)
	if !st.created {
		s.hub.sendToMany(audience, map[string]any{"type": "call.peer_joined", "room": key, "user": me})
		return nil
	}
	s.ringCall(me, key, st.video, group, without(audience, me.ID), groupID == 0)
	return nil
}

// ringCall tells everyone who may join that a call started. A one-to-one
// call that reaches nobody ends at once, and the caller hears why.
func (s *server) ringCall(me *User, key string, video bool, group map[string]any, targets []int64, oneToOne bool) {
	ring := withGroup(map[string]any{"type": "call.ring", "room": key, "video": video, "from_user": me}, group)
	reached := false
	for _, uid := range targets {
		reached = s.hub.sendTo(uid, ring) || reached
	}
	if oneToOne && !reached {
		s.hub.sendTo(me.ID, map[string]any{"type": "call.unavailable", "room": key, "user_id": targets[0]})
		s.leaveCall(me.ID, key)
	}
}

// ---- leaving

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
	audience := s.callAudience(r.groupID, r.peers)
	s.hub.sendToMany(audience, map[string]any{"type": "call.peer_left", "room": key, "user_id": userID})
	if empty {
		s.hub.sendToMany(audience, map[string]any{"type": "call.ended", "room": key})
	}
}

// roomsOf lists the rooms a user holds a seat in; with conn set, only seats
// taken from that connection.
func (c *callRooms) roomsOf(userID int64, conn *client) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var keys []string
	for k, r := range c.rooms {
		if m := r.members[userID]; m != nil && (conn == nil || m.conn == conn) {
			keys = append(keys, k)
		}
	}
	return keys
}

// leaveOtherCalls drops the user from every room except `keep`.
func (s *server) leaveOtherCalls(userID int64, keep string) {
	for _, k := range without(s.calls.roomsOf(userID, nil), keep) {
		s.leaveCall(userID, k)
	}
}

// leaveCallsOf frees the seats a dropped connection still holds (the user did
// not re-assert them from another connection).
func (s *server) leaveCallsOf(c *client) {
	for _, k := range s.calls.roomsOf(c.user.ID, c) {
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
	if r == nil || members == nil || !slicesContains(s.callAudience(r.groupID, r.peers), me.ID) {
		return
	}
	s.hub.sendToMany(members, map[string]any{"type": "call.declined", "room": key, "user_id": me.ID, "reason": reason})
}

func slicesContains(ids []int64, id int64) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}
