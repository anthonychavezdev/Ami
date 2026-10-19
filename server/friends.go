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
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return "none", nil
	case err != nil:
		return "", err
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

// friendIDs lists a user's accepted friends.
func (s *server) friendIDs(userID int64) []int64 {
	return queryIDs(s.db, `SELECT CASE WHEN from_id = ?1 THEN to_id ELSE from_id END FROM friendships
		WHERE status = 'accepted' AND (from_id = ?1 OR to_id = ?1)`, userID)
}

func (s *server) listFriends(w http.ResponseWriter, r *http.Request, me *User) error {
	users, err := queryAll(s.db, scanUser, `SELECT `+userCols+` FROM users u WHERE u.id IN (
		SELECT CASE WHEN from_id = ?1 THEN to_id ELSE from_id END FROM friendships
		WHERE status = 'accepted' AND (from_id = ?1 OR to_id = ?1)) ORDER BY u.display_name`, me.ID)
	if err != nil {
		return err
	}
	return ok(w, users)
}

func (s *server) listFriendRequests(w http.ResponseWriter, r *http.Request, me *User) error {
	incoming, err := queryAll(s.db, scanUser, `SELECT `+userCols+` FROM friendships f JOIN users u ON u.id = f.from_id
		WHERE f.to_id = ? AND f.status = 'pending' ORDER BY f.created_at DESC`, me.ID)
	if err != nil {
		return err
	}
	outgoing, err := queryAll(s.db, scanUser, `SELECT `+userCols+` FROM friendships f JOIN users u ON u.id = f.to_id
		WHERE f.from_id = ? AND f.status = 'pending' ORDER BY f.created_at DESC`, me.ID)
	if err != nil {
		return err
	}
	return ok(w, map[string][]User{"incoming": incoming, "outgoing": outgoing})
}

// friendChanged tells both people where their friendship stands now.
func (s *server) friendChanged(me, other *User, mine, theirs string) {
	s.hub.sendTo(me.ID, friendChangedEvent{Type: "friend.changed", User: other, Status: mine})
	s.hub.sendTo(other.ID, friendChangedEvent{Type: "friend.changed", User: me, Status: theirs})
}

// friendStatusResponse answers a friendship change with the new status.
func friendStatusResponse(w http.ResponseWriter, status string) error {
	return ok(w, struct {
		FriendStatus string `json:"friend_status"`
	}{status})
}

func (s *server) sendFriendRequest(w http.ResponseWriter, r *http.Request, me *User) error {
	other, err := pathID(r, "id")
	if err != nil {
		return err
	}
	if other == me.ID {
		return badRequest("cannot friend yourself")
	}
	target, err := s.userByID(other)
	if err != nil {
		return orNotFound(err, "user not found")
	}
	status, err := s.friendStatus(me.ID, other)
	if err != nil {
		return err
	}
	switch status {
	case "friends":
		return conflict("already friends")
	case "outgoing":
		return conflict("request already sent")
	case "incoming":
		return s.acceptFriend(w, me, target) // they already asked us
	}
	if _, err := s.db.Exec(`INSERT INTO friendships(from_id, to_id, status, created_at) VALUES (?, ?, 'pending', ?)`, me.ID, other, nowString()); err != nil {
		return err
	}
	s.friendChanged(me, &target, "outgoing", "incoming")
	return friendStatusResponse(w, "outgoing")
}

func (s *server) acceptFriendRequest(w http.ResponseWriter, r *http.Request, me *User) error {
	other, err := pathID(r, "id")
	if err != nil {
		return err
	}
	requester, err := s.userByID(other)
	if err != nil {
		return orNotFound(err, "user not found")
	}
	return s.acceptFriend(w, me, requester)
}

func (s *server) acceptFriend(w http.ResponseWriter, me *User, requester User) error {
	res, err := s.db.Exec(`UPDATE friendships SET status = 'accepted' WHERE from_id = ? AND to_id = ? AND status = 'pending'`, requester.ID, me.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return notFound("no pending request from that user")
	}
	s.friendChanged(me, &requester, "friends", "friends")
	return friendStatusResponse(w, "friends")
}

// removeFriend cancels an outgoing request, rejects an incoming one, or unfriends.
func (s *server) removeFriend(w http.ResponseWriter, r *http.Request, me *User) error {
	other, err := pathID(r, "id")
	if err != nil {
		return err
	}
	res, err := s.db.Exec(`DELETE FROM friendships WHERE (from_id = ?1 AND to_id = ?2) OR (from_id = ?2 AND to_id = ?1)`, me.ID, other)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		if target, err := s.userByID(other); err == nil {
			s.friendChanged(me, &target, "none", "none")
		}
	}
	return friendStatusResponse(w, "none")
}
