package main

import (
	"encoding/hex"
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
	if err := s.storeUpload(name, head, body); err != nil {
		return err
	}
	_ = rc.SetWriteDeadline(time.Now().Add(transferIdle)) // the request's own write deadline may be hours gone
	return created(w, s.attachmentFor(mime, name))
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
func (s *server) storeUpload(name string, head []byte, rest io.Reader) error {
	path := filepath.Join(s.mediaDir, name)
	out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err = out.Write(head); err == nil {
		_, err = io.Copy(out, rest)
	}
	out.Close()
	if err == nil {
		return nil
	}
	os.Remove(path)
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		return tooLarge()
	}
	if errors.Is(err, syscall.ENOSPC) {
		return fail(http.StatusInsufficientStorage, "no space left for uploads")
	}
	return err
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
