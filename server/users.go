package main

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
)

func (s *server) searchUsers(w http.ResponseWriter, r *http.Request, me *User) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	like := "%" + strings.NewReplacer("%", "\\%", "_", "\\_").Replace(q) + "%"
	rows, err := s.db.Query(`SELECT `+userCols+` FROM users u
		WHERE u.id != ? AND (u.username LIKE ? ESCAPE '\' OR u.display_name LIKE ? ESCAPE '\')
		ORDER BY u.username LIMIT 25`, me.ID, like, like)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, u)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) getUser(w http.ResponseWriter, r *http.Request, me *User) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	u, err := s.userByID(id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, u)
}
