package main

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
)

type Group struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	OwnerID     int64  `json:"owner_id"`
	Private     bool   `json:"private"` // invite only: hidden from discovery, the admin adds members
	CreatedAt   string `json:"created_at"`
	MemberCount int    `json:"member_count"`
	MyRole      string `json:"my_role"` // admin | member | "" (not a member)
}

type GroupMember struct {
	User
	Role string `json:"role"`
}

// groupSelect expects the viewer's user id as its first bound parameter.
const groupSelect = `
SELECT g.id, g.name, g.description, g.owner_id, g.private, g.created_at,
       (SELECT COUNT(*) FROM group_members m WHERE m.group_id = g.id),
       COALESCE((SELECT role FROM group_members m WHERE m.group_id = g.id AND m.user_id = ?), '')
FROM groups g
`

func scanGroup(sc interface{ Scan(...any) error }) (Group, error) {
	var g Group
	err := sc.Scan(&g.ID, &g.Name, &g.Description, &g.OwnerID, &g.Private, &g.CreatedAt, &g.MemberCount, &g.MyRole)
	return g, err
}

func (s *server) queryGroups(viewer int64, tail string, args ...any) ([]Group, error) {
	rows, err := s.db.Query(groupSelect+tail, append([]any{viewer}, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Group{}
	for rows.Next() {
		g, err := scanGroup(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// groupRole returns "admin", "member" or "" for a user in a group.
func (s *server) groupRole(groupID, userID int64) (string, error) {
	var role string
	err := s.db.QueryRow(`SELECT role FROM group_members WHERE group_id = ? AND user_id = ?`, groupID, userID).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return role, err
}

// listGroups: ?mine=1 lists groups I belong to; otherwise discover all (?q= filters by name).
func (s *server) listGroups(w http.ResponseWriter, r *http.Request, me *User) {
	var (
		groups []Group
		err    error
	)
	if r.URL.Query().Get("mine") == "1" {
		groups, err = s.queryGroups(me.ID, `WHERE g.id IN (SELECT group_id FROM group_members WHERE user_id = ?) ORDER BY g.name`, me.ID)
	} else {
		like := "%" + strings.TrimSpace(r.URL.Query().Get("q")) + "%"
		groups, err = s.queryGroups(me.ID, `WHERE g.private = 0 AND g.name LIKE ? ORDER BY g.name LIMIT 50`, like)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, groups)
}

// friendsToAdd checks a list of users the caller wants in a group: each must
// be a friend (no adding strangers to chats). Duplicates and the caller are dropped.
func (s *server) friendsToAdd(w http.ResponseWriter, me *User, ids []int64) ([]int64, bool) {
	if len(ids) > 100 {
		writeError(w, http.StatusBadRequest, "too many members at once")
		return nil, false
	}
	seen := map[int64]bool{me.ID: true}
	out := []int64{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		if ok, err := s.areFriends(me.ID, id); err != nil || !ok {
			writeError(w, http.StatusBadRequest, "you can only add friends")
			return nil, false
		}
		out = append(out, id)
	}
	return out, true
}

// addMember puts a user in a group as a member, with the chat read up to now
// (what was said before they came in does not count as unread).
func addMember(tx *sql.Tx, groupID, userID int64, ts string) error {
	if _, err := tx.Exec(`INSERT OR IGNORE INTO group_members(group_id, user_id, role, joined_at) VALUES (?, ?, 'member', ?)`, groupID, userID, ts); err != nil {
		return err
	}
	_, err := tx.Exec(`INSERT OR REPLACE INTO group_reads(user_id, group_id, last_read_id)
		VALUES (?1, ?2, COALESCE((SELECT MAX(id) FROM messages WHERE group_id = ?2), 0))`, userID, groupID)
	return err
}

// createGroup makes a group with the caller as owner and admin. {private:
// true} makes an invite-only group chat; {member_ids} adds friends right away.
func (s *server) createGroup(w http.ResponseWriter, r *http.Request, me *User) {
	var in struct {
		Name        string  `json:"name"`
		Description string  `json:"description"`
		Private     bool    `json:"private"`
		MemberIDs   []int64 `json:"member_ids"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Description = strings.TrimSpace(in.Description)
	if !clampLen(in.Name, 60) || len(in.Description) > 500 {
		writeError(w, http.StatusBadRequest, "name must be 1-60 characters, description up to 500")
		return
	}
	members, ok := s.friendsToAdd(w, me, in.MemberIDs)
	if !ok {
		return
	}
	tx, err := s.db.Begin()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()
	ts := nowString()
	res, err := tx.Exec(`INSERT INTO groups(name, description, owner_id, private, created_at) VALUES (?, ?, ?, ?, ?)`, in.Name, in.Description, me.ID, in.Private, ts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	id, _ := res.LastInsertId()
	if _, err := tx.Exec(`INSERT INTO group_members(group_id, user_id, role, joined_at) VALUES (?, ?, 'admin', ?)`, id, me.ID, ts); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, uid := range members {
		if err := addMember(tx, id, uid, ts); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.respondGroup(w, http.StatusCreated, me.ID, id)
}

func (s *server) respondGroup(w http.ResponseWriter, code int, viewer, id int64) {
	g, err := scanGroup(s.db.QueryRow(groupSelect+`WHERE g.id = ?`, viewer, id))
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "group not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	rows, err := s.db.Query(`SELECT `+userCols+`, m.role FROM group_members m JOIN users u ON u.id = m.user_id
		WHERE m.group_id = ? ORDER BY m.role = 'admin' DESC, m.joined_at ASC`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	members := []GroupMember{}
	for rows.Next() {
		var m GroupMember
		if err := rows.Scan(&m.ID, &m.Username, &m.DisplayName, &m.CreatedAt, &m.AvatarURL, &m.Pronouns, &m.AvatarCrop, &m.Role); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		members = append(members, m)
	}
	writeJSON(w, code, map[string]any{"group": g, "members": members, "call": s.calls.participants(groupRoomKey(id))})
}

// addGroupMembers lets the admin add friends: POST {user_ids: [...]}.
func (s *server) addGroupMembers(w http.ResponseWriter, r *http.Request, me *User) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		UserIDs []int64 `json:"user_ids"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	role, err := s.groupRole(id, me.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if role != "admin" {
		writeError(w, http.StatusForbidden, "admin only")
		return
	}
	members, ok := s.friendsToAdd(w, me, in.UserIDs)
	if !ok {
		return
	}
	tx, err := s.db.Begin()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()
	ts := nowString()
	for _, uid := range members {
		if err := addMember(tx, id, uid, ts); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	g := s.groupSummary(id)
	for _, uid := range members {
		s.hub.sendTo(uid, map[string]any{"type": "group.added", "group": g, "by": me})
	}
	s.respondGroup(w, http.StatusOK, me.ID, id)
}

// dropMember finishes a departure: the chat's read marker goes, the user is
// taken out of a running call, and their badge is recomputed.
func (s *server) dropMember(groupID, userID int64) {
	_, _ = s.db.Exec(`DELETE FROM group_reads WHERE user_id = ? AND group_id = ?`, userID, groupID)
	s.leaveCall(userID, groupRoomKey(groupID))
	unread, _ := s.unreadMessages(userID)
	s.hub.sendTo(userID, map[string]any{"type": "messages.read", "group_id": groupID, "unread": unread})
}

func (s *server) getGroupHandler(w http.ResponseWriter, r *http.Request, me *User) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	s.respondGroup(w, http.StatusOK, me.ID, id)
}

func (s *server) deleteGroup(w http.ResponseWriter, r *http.Request, me *User) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var owner int64
	if err := s.db.QueryRow(`SELECT owner_id FROM groups WHERE id = ?`, id).Scan(&owner); err != nil || owner != me.ID {
		writeError(w, http.StatusForbidden, "only the owner can delete the group")
		return
	}
	members := s.groupMemberIDs(id)
	s.endGroupCall(id)
	if _, err := s.db.Exec(`DELETE FROM groups WHERE id = ?`, id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, uid := range members { // the group chat is gone with it: refresh badges
		unread, _ := s.unreadMessages(uid)
		s.hub.sendTo(uid, map[string]any{"type": "messages.read", "group_id": id, "unread": unread})
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) joinGroup(w http.ResponseWriter, r *http.Request, me *User) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var private bool
	if err := s.db.QueryRow(`SELECT private FROM groups WHERE id = ?`, id).Scan(&private); err != nil {
		writeError(w, http.StatusNotFound, "group not found")
		return
	}
	if private {
		writeError(w, http.StatusForbidden, "this group is invite only")
		return
	}
	tx, err := s.db.Begin()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()
	if err := addMember(tx, id, me.ID, nowString()); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.respondGroup(w, http.StatusOK, me.ID, id)
}

func (s *server) leaveGroup(w http.ResponseWriter, r *http.Request, me *User) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var owner int64
	if err := s.db.QueryRow(`SELECT owner_id FROM groups WHERE id = ?`, id).Scan(&owner); err != nil {
		writeError(w, http.StatusNotFound, "group not found")
		return
	}
	if owner == me.ID {
		writeError(w, http.StatusBadRequest, "the owner cannot leave; delete the group instead")
		return
	}
	if _, err := s.db.Exec(`DELETE FROM group_members WHERE group_id = ? AND user_id = ?`, id, me.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.dropMember(id, me.ID)
	s.respondGroup(w, http.StatusOK, me.ID, id)
}

func (s *server) groupPosts(w http.ResponseWriter, r *http.Request, me *User) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	before, limit := pagination(r)
	posts, err := s.queryPosts(me.ID, `WHERE p.group_id = ? AND p.id < ? ORDER BY p.id DESC LIMIT ?`, id, before, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, posts)
}

// removeGroupMember lets the admin kick a non-owner member.
func (s *server) removeGroupMember(w http.ResponseWriter, r *http.Request, me *User) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	uid, ok := pathID(w, r, "uid")
	if !ok {
		return
	}
	role, err := s.groupRole(id, me.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if role != "admin" {
		writeError(w, http.StatusForbidden, "admin only")
		return
	}
	var owner int64
	_ = s.db.QueryRow(`SELECT owner_id FROM groups WHERE id = ?`, id).Scan(&owner)
	if uid == owner {
		writeError(w, http.StatusBadRequest, "cannot remove the owner")
		return
	}
	if _, err := s.db.Exec(`DELETE FROM group_members WHERE group_id = ? AND user_id = ?`, id, uid); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.dropMember(id, uid)
	s.respondGroup(w, http.StatusOK, me.ID, id)
}
