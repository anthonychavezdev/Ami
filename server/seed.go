package main

import (
	"database/sql"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// seedIfEmpty inserts the demo content when the database has no users yet.
func (s *server) seedIfEmpty() error {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	err := s.inTx(func(tx *sql.Tx) error {
		sd := &seeder{tx: tx, base: now(), mediaDir: s.mediaDir, users: map[string]int64{}, groups: map[string]int64{}}
		for _, step := range []func() error{sd.insertUsers, sd.insertFriendships, sd.insertGroups, sd.writePictures, sd.insertPosts, sd.insertMessages, sd.markRead} {
			if err := step(); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	names := mapSlice(demoUsers, func(u struct{ username, name string }) string { return u.username })
	log.Printf("dev: seeded demo data; log in as %s (password %q)", strings.Join(names, ", "), demoPassword)
	return nil
}

// seeder inserts the demo content step by step, keeping the ids it creates.
type seeder struct {
	tx       *sql.Tx
	base     time.Time
	mediaDir string
	users    map[string]int64 // username -> id
	groups   map[string]int64 // group name -> id
}

// ts is a timestamp `ago` before the seed time.
func (sd *seeder) ts(ago time.Duration) string { return sd.base.Add(-ago).Format(time.RFC3339Nano) }

func (sd *seeder) insertUsers() error {
	for i, u := range demoUsers {
		salt := randomBytes(16)
		res, err := sd.tx.Exec(`INSERT INTO users(username, display_name, password_hash, salt, password_iterations, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
			u.username, u.name, hashPassword(demoPassword, salt, pbkdf2Iterations), salt, pbkdf2Iterations, sd.ts(time.Duration(14-i)*24*hour))
		if err != nil {
			return err
		}
		sd.users[u.username], _ = res.LastInsertId()
		if p, ok := demoProfiles[u.username]; ok {
			if _, err := sd.tx.Exec(`UPDATE users SET pronouns = ?, bio = ? WHERE id = ?`, p[0], p[1], sd.users[u.username]); err != nil {
				return err
			}
		}
	}
	return nil
}

func (sd *seeder) insertFriendships() error {
	for _, f := range demoFriendships {
		status := map[bool]string{true: "accepted", false: "pending"}[f.accepted]
		if _, err := sd.tx.Exec(`INSERT INTO friendships(from_id, to_id, status, created_at) VALUES (?, ?, ?, ?)`,
			sd.users[f.from], sd.users[f.to], status, sd.ts(6*24*hour)); err != nil {
			return err
		}
	}
	return nil
}

func (sd *seeder) insertGroups() error {
	for _, g := range demoGroups {
		res, err := sd.tx.Exec(`INSERT INTO groups(name, description, owner_id, private, created_at) VALUES (?, ?, ?, ?, ?)`,
			g.name, g.description, sd.users[g.owner], g.private, sd.ts(6*24*hour))
		if err != nil {
			return err
		}
		id, _ := res.LastInsertId()
		sd.groups[g.name] = id
		if _, err := sd.tx.Exec(`INSERT INTO group_members(group_id, user_id, role, joined_at) VALUES (?, ?, 'admin', ?)`, id, sd.users[g.owner], sd.ts(6*24*hour)); err != nil {
			return err
		}
		for _, m := range g.members {
			if _, err := sd.tx.Exec(`INSERT INTO group_members(group_id, user_id, role, joined_at) VALUES (?, ?, 'member', ?)`, id, sd.users[m], sd.ts(5*24*hour)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (sd *seeder) writePictures() error {
	for name, c := range demoImages {
		if err := writeGradient(filepath.Join(sd.mediaDir, name), c[0], c[1]); err != nil {
			return err
		}
	}
	return nil
}

// groupIDOf is the group a post or message belongs to, or NULL.
func (sd *seeder) groupIDOf(name string) any {
	if name == "" {
		return nil
	}
	return sd.groups[name]
}

func (sd *seeder) insertPosts() error {
	for _, p := range demoPosts {
		atts := p.attachments
		if atts == nil {
			atts = []Attachment{}
		}
		res, err := sd.tx.Exec(`INSERT INTO posts(author_id, group_id, body, attachments, created_at) VALUES (?, ?, ?, ?, ?)`,
			sd.users[p.author], sd.groupIDOf(p.group), p.body, jsonString(atts), sd.ts(p.ago))
		if err != nil {
			return err
		}
		postID, _ := res.LastInsertId()
		if err := sd.insertComments(postID, p); err != nil {
			return err
		}
		for user, emoji := range p.reactions {
			if _, err := sd.tx.Exec(`INSERT INTO reactions(post_id, user_id, emoji) VALUES (?, ?, ?)`, postID, sd.users[user], emoji); err != nil {
				return err
			}
		}
	}
	return nil
}

func (sd *seeder) insertComments(postID int64, p demoPost) error {
	for i, c := range p.comments {
		if _, err := sd.tx.Exec(`INSERT INTO comments(post_id, author_id, body, created_at) VALUES (?, ?, ?, ?)`,
			postID, sd.users[c.author], c.body, sd.ts(p.ago-time.Duration(i+1)*7*time.Minute)); err != nil {
			return err
		}
	}
	return nil
}

func (sd *seeder) insertMessages() error {
	for _, m := range demoMessages {
		atts := []Attachment{}
		if m.image != "" {
			atts = []Attachment{demoImage(m.image)}
		}
		if _, err := sd.tx.Exec(`INSERT INTO messages(sender_id, recipient_id, body, attachments, created_at) VALUES (?, ?, ?, ?, ?)`,
			sd.users[m.from], sd.users[m.to], m.body, jsonString(atts), sd.ts(m.ago)); err != nil {
			return err
		}
	}
	for _, m := range demoGroupMessages {
		if _, err := sd.tx.Exec(`INSERT INTO messages(sender_id, group_id, body, attachments, created_at) VALUES (?, ?, ?, '[]', ?)`,
			sd.users[m.from], sd.groups[m.group], m.body, sd.ts(m.ago)); err != nil {
			return err
		}
	}
	return nil
}

// markRead: everything is read except Luis's latest message to Ana and
// Mateo's in the football group, so her Messages tab shows badges.
func (sd *seeder) markRead() error {
	statements := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO conversation_reads(user_id, peer_id, last_read_id)
			SELECT recipient_id, sender_id, MAX(id) FROM messages WHERE group_id IS NULL GROUP BY recipient_id, sender_id`, nil},
		{`UPDATE conversation_reads SET last_read_id =
			(SELECT id FROM messages WHERE sender_id = ?2 AND recipient_id = ?1 ORDER BY id DESC LIMIT 1 OFFSET 1)
			WHERE user_id = ?1 AND peer_id = ?2`, []any{sd.users["ana"], sd.users["luis"]}},
		{`INSERT INTO group_reads(user_id, group_id, last_read_id)
			SELECT gm.user_id, gm.group_id, COALESCE((SELECT MAX(id) FROM messages WHERE group_id = gm.group_id), 0) FROM group_members gm`, nil},
		{`UPDATE group_reads SET last_read_id =
			(SELECT id FROM messages WHERE group_id = ?2 ORDER BY id DESC LIMIT 1 OFFSET 1)
			WHERE user_id = ?1 AND group_id = ?2`, []any{sd.users["ana"], sd.groups["Fútbol los domingos"]}},
	}
	for _, st := range statements {
		if _, err := sd.tx.Exec(st.sql, st.args...); err != nil {
			return err
		}
	}
	return nil
}

// writeGradient renders a vertical gradient PNG (1200x900) at path.
func writeGradient(path string, from, to color.RGBA) error {
	const w, h = 1200, 900
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		c := blend(from, to, float64(y)/float64(h-1))
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// blend mixes two colours; t goes from 0 (from) to 1 (to).
func blend(from, to color.RGBA, t float64) color.RGBA {
	mix := func(a, b uint8) uint8 { return uint8(float64(a)*(1-t) + float64(b)*t) }
	return color.RGBA{R: mix(from.R, to.R), G: mix(from.G, to.G), B: mix(from.B, to.B), A: 255}
}
