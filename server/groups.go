package main

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
)

type Group struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	OwnerID     string `json:"owner_id"`
	Private     bool   `json:"private"` // invite only: hidden from discovery, the admin adds members
	CreatedAt   string `json:"created_at"`
	MemberCount int    `json:"member_count"`
	MyRole      string `json:"my_role"` // admin | member | "" (not a member)
}

type GroupMember struct {
	User
	Role string `json:"role"`
}

// GroupDetail is the answer of the group routes: the group, who is in it and
// who is in its call right now.
type GroupDetail struct {
	Group   Group         `json:"group"`
	Members []GroupMember `json:"members"`
	Call    []string      `json:"call"`
}

// ---- reading groups

// groupSelect expects the viewer's user id as its first bound parameter.
const groupSelect = `
SELECT g.id, g.name, g.description, g.owner_id, g.private, g.created_at,
       (SELECT COUNT(*) FROM group_members m WHERE m.group_id = g.id),
       COALESCE((SELECT role FROM group_members m WHERE m.group_id = g.id AND m.user_id = ?), '')
FROM groups g
`

func scanGroup(sc scanner) (Group, error) {
	var g Group
	err := sc.Scan(&g.ID, &g.Name, &g.Description, &g.OwnerID, &g.Private, &g.CreatedAt, &g.MemberCount, &g.MyRole)
	return g, err
}

func scanGroupMember(sc scanner) (GroupMember, error) {
	var m GroupMember
	err := sc.Scan(fields(m.User.fields(), []any{&m.Role})...)
	return m, err
}

func (s *server) queryGroups(viewer string, tail string, args ...any) ([]Group, error) {
	return queryAll(s.db, scanGroup, groupSelect+tail, append([]any{viewer}, args...)...)
}

// groupRole returns "admin", "member" or "" for a user in a group.
func (s *server) groupRole(groupID, userID string) (string, error) {
	var role string
	err := s.db.QueryRow(`SELECT role FROM group_members WHERE group_id = ? AND user_id = ?`, groupID, userID).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return role, err
}

// groupVisible says whether a viewer may look at a group: anyone signed in
// for a public one, members only for a private one. A missing group counts as
// visible, so the caller's own lookup answers the 404.
func (s *server) groupVisible(groupID, viewer string) (bool, error) {
	var private bool
	err := s.db.QueryRow(`SELECT private FROM groups WHERE id = ?`, groupID).Scan(&private)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !private) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	role, err := s.groupRole(groupID, viewer)
	return role != "", err
}

// viewingGroup guards a group route: a private group is "not found" to a
// non-member, so neither its existence nor its members show.
func (s *server) viewingGroup(h authedHandler) authedHandler {
	return func(w http.ResponseWriter, r *http.Request, me *User) error {
		ok, err := s.groupVisible(r.PathValue("id"), me.ID)
		if err != nil {
			return err
		}
		if !ok {
			return notFound("group not found")
		}
		return h(w, r, me)
	}
}

func (s *server) requireAdmin(groupID, userID string) error {
	role, err := s.groupRole(groupID, userID)
	if err != nil {
		return err
	}
	if role != "admin" {
		return forbidden("admin only")
	}
	return nil
}

func (s *server) groupOwner(id string) (string, error) {
	var owner string
	err := s.db.QueryRow(`SELECT owner_id FROM groups WHERE id = ?`, id).Scan(&owner)
	return owner, orNotFound(err, "group not found")
}

func (s *server) groupMemberIDs(groupID string) []string {
	return queryIDs(s.db, `SELECT user_id FROM group_members WHERE group_id = ?`, groupID)
}

// groupSummary is what call and message events say about a group.
func (s *server) groupSummary(id string) groupRef {
	g := groupRef{ID: id}
	_ = s.db.QueryRow(`SELECT name FROM groups WHERE id = ?`, id).Scan(&g.Name)
	return g
}

// listGroups: ?mine=1 lists groups I belong to; otherwise discover public
// groups (?q= filters by name).
func (s *server) listGroups(w http.ResponseWriter, r *http.Request, me *User) error {
	if r.URL.Query().Get("mine") == "1" {
		return s.respondGroups(w, me.ID, `WHERE g.id IN (SELECT group_id FROM group_members WHERE user_id = ?) ORDER BY g.name`, me.ID)
	}
	like := "%" + strings.TrimSpace(r.URL.Query().Get("q")) + "%"
	return s.respondGroups(w, me.ID, `WHERE g.private = 0 AND g.name LIKE ? ORDER BY g.name LIMIT 50`, like)
}

func (s *server) respondGroups(w http.ResponseWriter, viewer string, tail string, args ...any) error {
	groups, err := s.queryGroups(viewer, tail, args...)
	if err != nil {
		return err
	}
	return ok(w, groups)
}

// respondGroup answers with the group, its members and who is in its call.
func (s *server) respondGroup(w http.ResponseWriter, code int, viewer, id string) error {
	g, err := scanGroup(s.db.QueryRow(groupSelect+`WHERE g.id = ?`, viewer, id))
	if err != nil {
		return orNotFound(err, "group not found")
	}
	members, err := queryAll(s.db, scanGroupMember, `SELECT `+userCols+`, m.role FROM group_members m JOIN users u ON u.id = m.user_id
		WHERE m.group_id = ? ORDER BY m.role = 'admin' DESC, m.joined_at ASC`, id)
	if err != nil {
		return err
	}
	writeJSON(w, code, GroupDetail{Group: g, Members: members, Call: s.calls.participants(groupRoomKey(id))})
	return nil
}

func (s *server) getGroupHandler(w http.ResponseWriter, r *http.Request, me *User) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	return s.respondGroup(w, http.StatusOK, me.ID, id)
}

func (s *server) groupPosts(w http.ResponseWriter, r *http.Request, me *User) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	before, limit := pagination(r)
	posts, err := s.queryPosts(me.ID, `WHERE p.group_id = ? AND p.id < ? ORDER BY p.id DESC LIMIT ?`, id, before, limit)
	if err != nil {
		return err
	}
	return ok(w, posts)
}

// ---- creating and deleting groups

// groupInput is the body of POST /api/groups. {private: true} makes an
// invite-only group chat; {member_ids} adds friends right away.
type groupInput struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Private     bool     `json:"private"`
	MemberIDs   []string `json:"member_ids"`
}

func cleanGroupInput(in groupInput) (groupInput, error) {
	in.Name, in.Description = strings.TrimSpace(in.Name), strings.TrimSpace(in.Description)
	if !clampLen(in.Name, 60) || len(in.Description) > 500 {
		return in, badRequest("name must be 1-60 characters, description up to 500")
	}
	return in, nil
}

// createGroup makes a group with the caller as owner and admin.
func (s *server) createGroup(w http.ResponseWriter, r *http.Request, me *User) error {
	var raw groupInput
	if err := readJSON(w, r, &raw); err != nil {
		return err
	}
	in, err := cleanGroupInput(raw)
	if err != nil {
		return err
	}
	members, err := s.friendsToAdd(me.ID, in.MemberIDs)
	if err != nil {
		return err
	}
	id := newID()
	err = s.inTx(func(tx *sql.Tx) error {
		ts := nowString()
		if _, err := tx.Exec(`INSERT INTO groups(id, name, description, owner_id, private, created_at) VALUES (?, ?, ?, ?, ?, ?)`, id, in.Name, in.Description, me.ID, in.Private, ts); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO group_members(group_id, user_id, role, joined_at) VALUES (?, ?, 'admin', ?)`, id, me.ID, ts); err != nil {
			return err
		}
		return addMembers(tx, id, members, ts)
	})
	if err != nil {
		return err
	}
	return s.respondGroup(w, http.StatusCreated, me.ID, id)
}

func (s *server) deleteGroup(w http.ResponseWriter, r *http.Request, me *User) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	if owner, err := s.groupOwner(id); err != nil || owner != me.ID {
		return forbidden("only the owner can delete the group")
	}
	members := s.groupMemberIDs(id)
	s.endGroupCall(id)
	if _, err := s.db.Exec(`DELETE FROM groups WHERE id = ?`, id); err != nil {
		return err
	}
	for _, uid := range members { // the group chat is gone with it
		s.refreshBadge(uid, id)
	}
	return noContent(w)
}

// ---- membership

// friendsToAdd checks a list of users the caller wants in a group: each must
// be a friend (no adding strangers to chats). Duplicates and the caller are dropped.
func (s *server) friendsToAdd(me string, ids []string) ([]string, error) {
	if len(ids) > 100 {
		return nil, badRequest("too many members at once")
	}
	friends := map[string]bool{}
	for _, id := range s.friendIDs(me) {
		friends[id] = true
	}
	seen := map[string]bool{me: true}
	out := []string{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		if !friends[id] {
			return nil, badRequest("you can only add friends")
		}
		out = append(out, id)
	}
	return out, nil
}

// addMember puts a user in a group as a member, with the chat read up to now
// (what was said before they came in does not count as unread).
func addMember(tx *sql.Tx, groupID, userID string, ts string) error {
	if _, err := tx.Exec(`INSERT IGNORE INTO group_members(group_id, user_id, role, joined_at) VALUES (?, ?, 'member', ?)`, groupID, userID, ts); err != nil {
		return err
	}
	_, err := tx.Exec(`REPLACE INTO group_reads(user_id, group_id, last_read_id)
		VALUES (?, ?, COALESCE((SELECT MAX(id) FROM messages WHERE group_id = ?), ''))`, userID, groupID, groupID)
	return err
}

func addMembers(tx *sql.Tx, groupID string, userIDs []string, ts string) error {
	for _, uid := range userIDs {
		if err := addMember(tx, groupID, uid, ts); err != nil {
			return err
		}
	}
	return nil
}

func (s *server) joinGroup(w http.ResponseWriter, r *http.Request, me *User) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var private bool
	if err := s.db.QueryRow(`SELECT private FROM groups WHERE id = ?`, id).Scan(&private); err != nil {
		return notFound("group not found")
	}
	if private {
		return forbidden("this group is invite only")
	}
	if err := s.inTx(func(tx *sql.Tx) error { return addMember(tx, id, me.ID, nowString()) }); err != nil {
		return err
	}
	return s.respondGroup(w, http.StatusOK, me.ID, id)
}

// addGroupMembers lets the admin add friends: POST {user_ids: [...]}.
func (s *server) addGroupMembers(w http.ResponseWriter, r *http.Request, me *User) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		UserIDs []string `json:"user_ids"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	if err := s.requireAdmin(id, me.ID); err != nil {
		return err
	}
	members, err := s.friendsToAdd(me.ID, in.UserIDs)
	if err != nil {
		return err
	}
	if err := s.inTx(func(tx *sql.Tx) error { return addMembers(tx, id, members, nowString()) }); err != nil {
		return err
	}
	s.hub.sendToMany(members, groupAddedEvent{Type: "group.added", Group: s.groupSummary(id), By: me})
	return s.respondGroup(w, http.StatusOK, me.ID, id)
}

func (s *server) leaveGroup(w http.ResponseWriter, r *http.Request, me *User) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	owner, err := s.groupOwner(id)
	if err != nil {
		return err
	}
	if owner == me.ID {
		return badRequest("the owner cannot leave; delete the group instead")
	}
	if err := s.dropMember(id, me.ID); err != nil {
		return err
	}
	return s.respondGroup(w, http.StatusOK, me.ID, id)
}

// removeGroupMember lets the admin kick a non-owner member.
func (s *server) removeGroupMember(w http.ResponseWriter, r *http.Request, me *User) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	uid, err := pathID(r, "uid")
	if err != nil {
		return err
	}
	if err := s.requireAdmin(id, me.ID); err != nil {
		return err
	}
	if owner, _ := s.groupOwner(id); uid == owner {
		return badRequest("cannot remove the owner")
	}
	if err := s.dropMember(id, uid); err != nil {
		return err
	}
	return s.respondGroup(w, http.StatusOK, me.ID, id)
}

// dropMember takes a user out of a group: membership and read marker go, they
// leave the group's call, and their badge is recomputed.
func (s *server) dropMember(groupID, userID string) error {
	if _, err := s.db.Exec(`DELETE FROM group_members WHERE group_id = ? AND user_id = ?`, groupID, userID); err != nil {
		return err
	}
	_, _ = s.db.Exec(`DELETE FROM group_reads WHERE user_id = ? AND group_id = ?`, userID, groupID)
	s.leaveCall(userID, groupRoomKey(groupID))
	s.refreshBadge(userID, groupID)
	return nil
}

// refreshBadge tells a user's devices their unread total after a group chat
// stopped counting for them.
func (s *server) refreshBadge(userID, groupID string) {
	unread, _ := s.unreadMessages(userID)
	s.hub.sendTo(userID, chat{group: groupID}.readEvent(unread))
}
