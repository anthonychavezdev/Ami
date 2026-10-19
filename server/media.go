package main

import (
	"encoding/hex"
	"errors"
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
	maxUploadBytes = 64 << 20
	maxAttachments = 10
)

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

// uploadMedia stores one picture or video under a random name and returns the
// attachment to put on a post.
func (s *server) uploadMedia(w http.ResponseWriter, r *http.Request, me *User) error {
	if !s.uploadLimiter.allow(me.ID) {
		w.Header().Set("Retry-After", "60")
		return fail(http.StatusTooManyRequests, "too many uploads, try again in a minute")
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	file, header, err := r.FormFile("file")
	if err != nil {
		return badRequest(`multipart form with a "file" field required (max 64 MB)`)
	}
	defer file.Close()
	head := make([]byte, 512)
	n, _ := io.ReadFull(file, head)
	head = head[:n]
	mime := sniffMedia(head, header.Header.Get("Content-Type"))
	ext, known := mediaExt[mime]
	if !known {
		return fail(http.StatusUnsupportedMediaType, "only JPEG, PNG, GIF and WebP pictures or MP4, MOV and WebM videos are accepted")
	}
	name := hex.EncodeToString(randomBytes(16)) + ext
	if err := s.storeUpload(name, head, file); err != nil {
		return err
	}
	return created(w, s.attachmentFor(mime, name))
}

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
		return fail(http.StatusRequestEntityTooLarge, "file too large (max 64 MB)")
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
	http.ServeFile(w, r, filepath.Join(s.mediaDir, name))
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
		u, err := url.Parse(a.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return a, errors.New("links must be http(s) URLs")
		}
		return Attachment{Type: "link", URL: a.URL}, nil
	default:
		return a, errors.New("attachment type must be image, video or link")
	}
}
