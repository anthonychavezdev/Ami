package main

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Attachment is a picture, video or link carried by a post. Pictures and
// videos are uploaded to POST /api/media first and served from /media/.
type Attachment struct {
	Type   string `json:"type"` // image | video | link
	URL    string `json:"url"`  // /media/<name> for uploads, absolute for links
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
}

const (
	maxUploadGB    = 20
	maxAttachments = 10
	// transferIdle is how long an upload or download may stall before its
	// connection is dropped. While bytes keep moving there is no overall limit:
	// a 20 GB transfer takes hours on an ordinary link, far past the server's
	// read and write timeouts, which are sized for ordinary requests.
	transferIdle = 2 * time.Minute
)

// maxUploadBytes caps one upload; a variable so tests can lower it.
var maxUploadBytes int64 = maxUploadGB * 1_000_000_000 // decimal, the units phones and the app show

func tooLarge() error {
	return fail(http.StatusRequestEntityTooLarge, fmt.Sprintf("file too large (max %d GB)", maxUploadGB))
}

var mediaExt = map[string]string{
	"image/jpeg": ".jpg", "image/png": ".png", "image/gif": ".gif", "image/webp": ".webp",
	"video/mp4": ".mp4", "video/quicktime": ".mov", "video/webm": ".webm",
}

// uploadName is the file behind an upload URL (/media/<name>), if it is one.
func uploadName(url string) (string, bool) {
	name := strings.TrimPrefix(url, "/media/")
	return name, name != url && name != "" && !strings.ContainsAny(name, `/\`)
}

func (s *server) uploadExists(name string) bool {
	_, err := os.Stat(filepath.Join(s.mediaDir, name))
	return err == nil
}

// uploadMedia stores one picture or video, sent as the request body, under a
// random name and returns the attachment to put on a post. The body streams to
// disk as it arrives: nothing is held in memory or spooled to a temporary file.
func (s *server) uploadMedia(w http.ResponseWriter, r *http.Request, me *User) error {
	if !s.uploadLimiter.allow(me.ID) {
		w.Header().Set("Retry-After", "60")
		return fail(http.StatusTooManyRequests, "too many uploads, try again in a minute")
	}
	if r.ContentLength > maxUploadBytes {
		return tooLarge()
	}
	used, err := s.userUploads(me.ID)
	if err != nil {
		return err
	}
	if s.storageCap > 0 && r.ContentLength > 0 && used+r.ContentLength > s.storageCap {
		return storageFull(used, s.storageCap)
	}
	rc := http.NewResponseController(w)
	body := &pacedReader{Reader: http.MaxBytesReader(w, r.Body, maxUploadBytes)}
	body.extend = func() { _ = rc.SetReadDeadline(time.Now().Add(transferIdle)) }
	head := make([]byte, 512)
	n, _ := io.ReadFull(body, head)
	if n == 0 {
		return badRequest("send the picture or video as the request body")
	}
	head = head[:n]
	mime := sniffMedia(head, r.Header.Get("Content-Type"))
	ext, known := mediaExt[mime]
	if !known {
		return fail(http.StatusUnsupportedMediaType, "only JPEG, PNG, GIF and WebP pictures or MP4, MOV and WebM videos are accepted")
	}
	name := hex.EncodeToString(randomBytes(16)) + ext
	size, err := s.storeUpload(name, head, body)
	if err != nil {
		return err
	}
	if s.storageCap > 0 && used+size > s.storageCap { // a body sent without a length is only known now
		os.Remove(filepath.Join(s.mediaDir, name))
		return storageFull(used, s.storageCap)
	}
	if _, err := s.db.Exec(`INSERT INTO uploads(name, user_id, size, created_at) VALUES (?, ?, ?, ?)`, name, me.ID, size, nowString()); err != nil {
		os.Remove(filepath.Join(s.mediaDir, name))
		return err
	}
	_ = rc.SetWriteDeadline(time.Now().Add(transferIdle)) // the request's own write deadline may be hours gone
	return created(w, s.attachmentFor(mime, name))
}

// storageFull refuses an upload that would take an account past -user-storage-gb.
func storageFull(used, limit int64) error {
	return fail(http.StatusRequestEntityTooLarge, fmt.Sprintf("your storage is full: %s of %s used", formatBytes(used), formatBytes(limit)))
}

// formatBytes is decimal, like the app and the upload limit: 1 GB is a thousand million bytes.
func formatBytes(n int64) string {
	switch {
	case n >= 1_000_000_000:
		return fmt.Sprintf("%.1f GB", float64(n)/1e9)
	case n >= 1_000_000:
		return fmt.Sprintf("%.1f MB", float64(n)/1e6)
	case n >= 1_000:
		return fmt.Sprintf("%.1f kB", float64(n)/1e3)
	}
	return fmt.Sprintf("%d B", n)
}

// userUploads is what an account has uploaded so far, in bytes, attached or not.
func (s *server) userUploads(userID string) (int64, error) {
	var total int64
	err := s.db.QueryRow(`SELECT COALESCE(SUM(size), 0) FROM uploads WHERE user_id = ?`, userID).Scan(&total)
	return total, err
}

// recordOldUploads gives the files from before the uploads table an owner and
// a size: the author of a post or message that attached them, or the profile
// wearing them. It runs at every start and costs a lookup per known file.
func (s *server) recordOldUploads() error {
	known := map[string]bool{}
	for _, name := range queryIDs(s.db, `SELECT name FROM uploads`) {
		known[name] = true
	}
	record := func(owner, url string) {
		name, isUpload := uploadName(url)
		if !isUpload || known[name] {
			return
		}
		known[name] = true
		info, err := os.Stat(filepath.Join(s.mediaDir, name))
		if err != nil {
			return
		}
		_, _ = s.db.Exec(`INSERT IGNORE INTO uploads(name, user_id, size, created_at) VALUES (?, ?, ?, ?)`,
			name, owner, info.Size(), info.ModTime().UTC().Format(time.RFC3339Nano))
	}
	rows, err := s.db.Query(`SELECT id, avatar_url, cover_url FROM users`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id, avatar, cover string
		if err := rows.Scan(&id, &avatar, &cover); err != nil {
			rows.Close()
			return err
		}
		record(id, avatar)
		record(id, cover)
	}
	rows.Close()
	for _, query := range []string{`SELECT author_id, attachments FROM posts`, `SELECT sender_id, attachments FROM messages WHERE sender_id IS NOT NULL`} {
		rows, err := s.db.Query(query)
		if err != nil {
			return err
		}
		for rows.Next() {
			var owner, raw string
			if err := rows.Scan(&owner, &raw); err != nil {
				rows.Close()
				return err
			}
			var atts []Attachment
			if json.Unmarshal([]byte(raw), &atts) == nil {
				for _, a := range atts {
					record(owner, a.URL)
				}
			}
		}
		rows.Close()
	}
	return nil
}

// pacer calls extend as bytes pass through a transfer, at most once a second,
// so one that keeps moving is never cut off by the server's timeouts while one
// that stalls still is (see transferIdle).
type pacer struct {
	extend func()
	last   time.Time
}

func (p *pacer) tick() {
	if now := time.Now(); now.Sub(p.last) >= time.Second {
		p.last = now
		p.extend()
	}
}

// pacedReader paces a request body (an upload).
type pacedReader struct {
	io.Reader
	pacer
}

func (p *pacedReader) Read(b []byte) (int, error) {
	p.tick()
	return p.Reader.Read(b)
}

// pacedWriter paces a response (a download from /media/).
type pacedWriter struct {
	http.ResponseWriter
	pacer
}

func (p *pacedWriter) Write(b []byte) (int, error) {
	p.tick()
	return p.ResponseWriter.Write(b)
}

func (p *pacedWriter) Unwrap() http.ResponseWriter { return p.ResponseWriter }

// storeUpload writes the sniffed head and the rest of the body to a new file.
func (s *server) storeUpload(name string, head []byte, rest io.Reader) (int64, error) {
	path := filepath.Join(s.mediaDir, name)
	out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return 0, err
	}
	var copied int64
	if _, err = out.Write(head); err == nil {
		copied, err = io.Copy(out, rest)
	}
	out.Close()
	if err == nil {
		return int64(len(head)) + copied, nil
	}
	os.Remove(path)
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		return 0, tooLarge()
	}
	if errors.Is(err, syscall.ENOSPC) {
		return 0, fail(http.StatusInsufficientStorage, "no space left for uploads")
	}
	return 0, err
}

// attachmentFor describes a stored upload; pictures carry their size.
func (s *server) attachmentFor(mime, name string) Attachment {
	att := Attachment{Type: "image", URL: "/media/" + name}
	if strings.HasPrefix(mime, "video/") {
		att.Type = "video"
		return att
	}
	att.Width, att.Height = s.pictureSize(name)
	return att
}

// pictureSize decodes just the header of an uploaded picture; 0x0 when it
// cannot be decoded (WebP has no decoder in the standard library).
func (s *server) pictureSize(name string) (int, int) {
	f, err := os.Open(filepath.Join(s.mediaDir, name))
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}

// sniffMedia trusts the content over the declared type, except for MOV/MP4
// brands Go's sniffer doesn't know (iPhone videos), which must at least be an
// ISO media file.
func sniffMedia(head []byte, declared string) string {
	detected := http.DetectContentType(head)
	if _, ok := mediaExt[detected]; ok {
		return detected
	}
	declared, _, _ = strings.Cut(declared, ";")
	declared = strings.TrimSpace(declared)
	if strings.HasPrefix(declared, "video/") && len(head) >= 12 && string(head[4:8]) == "ftyp" {
		if _, ok := mediaExt[declared]; ok {
			return declared
		}
		return "video/mp4"
	}
	return detected
}

// validLink checks a URL someone typed, for a post's link attachment or a
// profile link: http or https, with a host, at most 2048 bytes.
func validLink(raw string) (*url.URL, error) {
	if len(raw) > 2048 {
		return nil, errors.New("link too long")
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("links must be http(s) URLs")
	}
	return u, nil
}

// serveMedia serves uploaded files. Names are random, so they need no auth and
// can be cached forever.
func (s *server) serveMedia(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" || strings.ContainsAny(name, `/\`) || strings.HasPrefix(name, ".") {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	// Uploads are only ever images and videos; if one is opened as a page it cannot run anything.
	w.Header().Set("Content-Security-Policy", "default-src 'none'; img-src 'self'; media-src 'self'; style-src 'unsafe-inline'; sandbox")
	// A 20 GB video outlasts the server's write timeout; keep extending it while bytes move.
	rc := http.NewResponseController(w)
	out := &pacedWriter{ResponseWriter: w}
	out.extend = func() { _ = rc.SetWriteDeadline(time.Now().Add(transferIdle)) }
	http.ServeFile(out, r, filepath.Join(s.mediaDir, name))
}

// cleanAttachments checks a post's attachments (uploads must exist in the
// media directory, links must be http(s) URLs) and returns them normalised.
func (s *server) cleanAttachments(atts []Attachment) ([]Attachment, error) {
	if len(atts) > maxAttachments {
		return nil, badRequest("too many attachments (max 10)")
	}
	out := make([]Attachment, 0, len(atts))
	for _, a := range atts {
		clean, err := s.cleanAttachment(a)
		if err != nil {
			return nil, badRequest(err.Error())
		}
		out = append(out, clean)
	}
	return out, nil
}

func (s *server) cleanAttachment(a Attachment) (Attachment, error) {
	if len(a.URL) > 2048 {
		return a, errors.New("attachment URL too long")
	}
	switch a.Type {
	case "image", "video":
		name, ok := uploadName(a.URL)
		if !ok {
			return a, errors.New("pictures and videos must be uploaded via /api/media")
		}
		if !s.uploadExists(name) {
			return a, errors.New("unknown media file " + name)
		}
		return a, nil
	case "link":
		if _, err := validLink(a.URL); err != nil {
			return a, err
		}
		return Attachment{Type: "link", URL: a.URL}, nil
	default:
		return a, errors.New("attachment type must be image, video or link")
	}
}

// ---- reactions on uploads: a picture or video in the fullscreen viewer

// mediaName is the upload named in the path, if it exists.
func (s *server) mediaName(r *http.Request) (string, error) {
	name := r.PathValue("name")
	if name == "" || len(name) > 64 || strings.ContainsAny(name, `/\`) || strings.HasPrefix(name, ".") {
		return "", badRequest("invalid media name")
	}
	if !s.uploadExists(name) {
		return "", notFound("media not found")
	}
	return name, nil
}

// respondMediaReactions answers who reacted to an upload with what; the
// viewer takes its counts and the viewer's own reaction from the list.
func (s *server) respondMediaReactions(w http.ResponseWriter, name string) error {
	return s.respondReactionGroups(w, `SELECT r.emoji, `+userCols+` FROM media_reactions r JOIN users u ON u.id = r.user_id WHERE r.media = ? ORDER BY r.emoji, u.display_name`, name)
}

func (s *server) listMediaReactions(w http.ResponseWriter, r *http.Request, me *User) error {
	name, err := s.mediaName(r)
	if err != nil {
		return err
	}
	return s.respondMediaReactions(w, name)
}

func (s *server) reactToMedia(w http.ResponseWriter, r *http.Request, me *User) error {
	var in struct {
		Emoji string `json:"emoji"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	emoji, err := validEmoji(in.Emoji)
	if err != nil {
		return err
	}
	name, err := s.mediaName(r)
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(`INSERT INTO media_reactions(media, user_id, emoji) VALUES (?, ?, ?)
		ON DUPLICATE KEY UPDATE emoji = VALUES(emoji)`, name, me.ID, emoji); err != nil {
		return err
	}
	return s.respondMediaReactions(w, name)
}

func (s *server) unreactToMedia(w http.ResponseWriter, r *http.Request, me *User) error {
	name, err := s.mediaName(r)
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(`DELETE FROM media_reactions WHERE media = ? AND user_id = ?`, name, me.ID); err != nil {
		return err
	}
	return s.respondMediaReactions(w, name)
}
