package main

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
)

type Post struct {
	ID        int64  `json:"id"`
	Author    User   `json:"author"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
}

const postSelect = `
SELECT p.id, p.body, p.created_at,
       ` + userCols + `
FROM posts p
JOIN users u ON u.id = p.author_id
`

func scanPost(sc interface{ Scan(...any) error }) (Post, error) {
	var p Post
	err := sc.Scan(&p.ID, &p.Body, &p.CreatedAt,
		&p.Author.ID, &p.Author.Username, &p.Author.DisplayName, &p.Author.CreatedAt)
	return p, err
}

// queryPosts runs postSelect + tail.
func (s *server) queryPosts(tail string, args ...any) ([]Post, error) {
	rows, err := s.db.Query(postSelect+tail, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	posts := []Post{}
	for rows.Next() {
		p, err := scanPost(rows)
		if err != nil {
			return nil, err
		}
		posts = append(posts, p)
	}
	return posts, rows.Err()
}

func (s *server) getPost(id int64) (Post, error) {
	return scanPost(s.db.QueryRow(postSelect+`WHERE p.id = ?`, id))
}

// feed: my own posts, newest first.
func (s *server) feed(w http.ResponseWriter, r *http.Request, me *User) {
	before, limit := pagination(r)
	posts, err := s.queryPosts(`WHERE p.author_id = ?1 AND p.id < ?2 ORDER BY p.id DESC LIMIT ?3`, me.ID, before, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, posts)
}

func (s *server) createPost(w http.ResponseWriter, r *http.Request, me *User) {
	var in struct {
		Body string `json:"body"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	in.Body = strings.TrimSpace(in.Body)
	if !clampLen(in.Body, 5000) {
		writeError(w, http.StatusBadRequest, "body must be 1-5000 characters")
		return
	}
	res, err := s.db.Exec(`INSERT INTO posts(author_id, body, created_at) VALUES (?, ?, ?)`,
		me.ID, in.Body, nowString())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	id, _ := res.LastInsertId()
	s.respondPost(w, http.StatusCreated, id)
}

func (s *server) respondPost(w http.ResponseWriter, code int, id int64) {
	p, err := s.getPost(id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "post not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, code, p)
}

func (s *server) getPostHandler(w http.ResponseWriter, r *http.Request, me *User) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	s.respondPost(w, http.StatusOK, id)
}

// deletePost: only the author may delete a post.
func (s *server) deletePost(w http.ResponseWriter, r *http.Request, me *User) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var author int64
	err := s.db.QueryRow(`SELECT author_id FROM posts WHERE id = ?`, id).Scan(&author)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "post not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if author != me.ID {
		writeError(w, http.StatusForbidden, "not allowed")
		return
	}
	if _, err := s.db.Exec(`DELETE FROM posts WHERE id = ?`, id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
