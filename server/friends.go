package main

import (
	"database/sql"
	"errors"
	"net/http"
)

// friendStatus describes the relationship from me's point of view.
func (s *server) friendStatus(me, other int64) (string, error) {
	if me == other {
		return "self", nil
	}
	var from int64
	var status string
	err := s.db.QueryRow(`SELECT from_id, status FROM friendships
		WHERE (from_id = ?1 AND to_id = ?2) OR (from_id = ?2 AND to_id = ?1)`, me, other).Scan(&from, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return "none", nil
	}
	if err != nil {
		return "", err
	}
	switch {
	case status == "accepted":
		return "friends", nil
	case from == me:
		return "outgoing", nil
	default:
		return "incoming", nil
	}
}

func (s *server) areFriends(a, b int64) (bool, error) {
	st, err := s.friendStatus(a, b)
	return st == "friends", err
}

func (s *server) queryUsers(query string, args ...any) ([]User, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *server) listFriends(w http.ResponseWriter, r *http.Request, me *User) {
	users, err := s.queryUsers(`SELECT `+userCols+` FROM users u WHERE u.id IN (
		SELECT CASE WHEN from_id = ?1 THEN to_id ELSE from_id END FROM friendships
		WHERE status = 'accepted' AND (from_id = ?1 OR to_id = ?1)) ORDER BY u.display_name`, me.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, users)
}

func (s *server) listFriendRequests(w http.ResponseWriter, r *http.Request, me *User) {
	incoming, err := s.queryUsers(`SELECT `+userCols+` FROM friendships f JOIN users u ON u.id = f.from_id
		WHERE f.to_id = ? AND f.status = 'pending' ORDER BY f.created_at DESC`, me.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	outgoing, err := s.queryUsers(`SELECT `+userCols+` FROM friendships f JOIN users u ON u.id = f.to_id
		WHERE f.from_id = ? AND f.status = 'pending' ORDER BY f.created_at DESC`, me.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string][]User{"incoming": incoming, "outgoing": outgoing})
}

func (s *server) sendFriendRequest(w http.ResponseWriter, r *http.Request, me *User) {
	other, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if other == me.ID {
		writeError(w, http.StatusBadRequest, "cannot friend yourself")
		return
	}
	target, err := s.userByID(other)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	status, err := s.friendStatus(me.ID, other)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	switch status {
	case "friends":
		writeError(w, http.StatusConflict, "already friends")
		return
	case "outgoing":
		writeError(w, http.StatusConflict, "request already sent")
		return
	case "incoming":
		// They already asked us: treat as accept.
		s.acceptFriend(w, me, target)
		return
	}
	if _, err := s.db.Exec(`INSERT INTO friendships(from_id, to_id, status, created_at) VALUES (?, ?, 'pending', ?)`,
		me.ID, other, nowString()); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"friend_status": "outgoing"})
}

func (s *server) acceptFriendRequest(w http.ResponseWriter, r *http.Request, me *User) {
	other, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	requester, err := s.userByID(other)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	s.acceptFriend(w, me, requester)
}

func (s *server) acceptFriend(w http.ResponseWriter, me *User, requester User) {
	res, err := s.db.Exec(`UPDATE friendships SET status = 'accepted' WHERE from_id = ? AND to_id = ? AND status = 'pending'`,
		requester.ID, me.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "no pending request from that user")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"friend_status": "friends"})
}

// removeFriend cancels an outgoing request, rejects an incoming one, or unfriends.
func (s *server) removeFriend(w http.ResponseWriter, r *http.Request, me *User) {
	other, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	_, err := s.db.Exec(`DELETE FROM friendships WHERE (from_id = ?1 AND to_id = ?2) OR (from_id = ?2 AND to_id = ?1)`, me.ID, other)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"friend_status": "none"})
}
