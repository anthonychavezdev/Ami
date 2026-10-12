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
SELECT g.id, g.name, g.description, g.owner_id, g.created_at,
       (SELECT COUNT(*) FROM group_members m WHERE m.group_id = g.id),
       COALESCE((SELECT role FROM group_members m WHERE m.group_id = g.id AND m.user_id = ?), '')
FROM groups g
`

func scanGroup(sc interface{ Scan(...any) error }) (Group, error) {
	var g Group
	err := sc.Scan(&g.ID, &g.Name, &g.Description, &g.OwnerID, &g.CreatedAt, &g.MemberCount, &g.MyRole)
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
		groups, err = s.queryGroups(me.ID, `WHERE g.name LIKE ? ORDER BY g.name LIMIT 50`, like)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, groups)
}

func (s *server) createGroup(w http.ResponseWriter, r *http.Request, me *User) {
	var in struct {
		Name        string `json:"name"`
		Description string `json:"description"`
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
	tx, err := s.db.Begin()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()
	ts := nowString()
	res, err := tx.Exec(`INSERT INTO groups(name, description, owner_id, created_at) VALUES (?, ?, ?, ?)`, in.Name, in.Description, me.ID, ts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	id, _ := res.LastInsertId()
	if _, err := tx.Exec(`INSERT INTO group_members(group_id, user_id, role, joined_at) VALUES (?, ?, 'admin', ?)`, id, me.ID, ts); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
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
	writeJSON(w, code, map[string]any{"group": g, "members": members})
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
	res, err := s.db.Exec(`DELETE FROM groups WHERE id = ? AND owner_id = ?`, id, me.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeError(w, http.StatusForbidden, "only the owner can delete the group")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) joinGroup(w http.ResponseWriter, r *http.Request, me *User) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var exists int
	if err := s.db.QueryRow(`SELECT 1 FROM groups WHERE id = ?`, id).Scan(&exists); err != nil {
		writeError(w, http.StatusNotFound, "group not found")
		return
	}
	if _, err := s.db.Exec(`INSERT OR IGNORE INTO group_members(group_id, user_id, role, joined_at) VALUES (?, ?, 'member', ?)`,
		id, me.ID, nowString()); err != nil {
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
	s.respondGroup(w, http.StatusOK, me.ID, id)
}
