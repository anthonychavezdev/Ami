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
	"strconv"
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

// uploadMedia stores one picture or video under a random name and returns the
// attachment to put on a post.
func (s *server) uploadMedia(w http.ResponseWriter, r *http.Request, me *User) {
	if !s.uploadLimiter.allow(strconv.FormatInt(me.ID, 10)) {
		w.Header().Set("Retry-After", "60")
		writeError(w, http.StatusTooManyRequests, "too many uploads, try again in a minute")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, `multipart form with a "file" field required (max 64 MB)`)
		return
	}
	defer file.Close()
	head := make([]byte, 512)
	n, _ := io.ReadFull(file, head)
	head = head[:n]
	mime := sniffMedia(head, header.Header.Get("Content-Type"))
	ext, ok := mediaExt[mime]
	if !ok {
		writeError(w, http.StatusUnsupportedMediaType, "only JPEG, PNG, GIF and WebP pictures or MP4, MOV and WebM videos are accepted")
		return
	}
	name := hex.EncodeToString(randomBytes(16)) + ext
	path := filepath.Join(s.mediaDir, name)
	out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_, err = out.Write(head)
	if err == nil {
		_, err = io.Copy(out, file)
	}
	out.Close()
	if err != nil {
		os.Remove(path)
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, "file too large (max 64 MB)")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	att := Attachment{Type: "image", URL: "/media/" + name}
	if strings.HasPrefix(mime, "video/") {
		att.Type = "video"
	} else if f, err := os.Open(path); err == nil {
		if cfg, _, err := image.DecodeConfig(f); err == nil {
			att.Width, att.Height = cfg.Width, cfg.Height
		}
		f.Close()
	}
	writeJSON(w, http.StatusCreated, att)
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

// validateAttachments checks a post's attachments: uploads must exist in the
// media directory and links must be http(s) URLs.
func (s *server) validateAttachments(atts []Attachment) error {
	if len(atts) > maxAttachments {
		return errors.New("too many attachments (max 10)")
	}
	for i := range atts {
		a := &atts[i]
		if len(a.URL) > 2048 {
			return errors.New("attachment URL too long")
		}
		switch a.Type {
		case "image", "video":
			name := strings.TrimPrefix(a.URL, "/media/")
			if name == a.URL || name == "" || strings.ContainsAny(name, `/\`) {
				return errors.New("pictures and videos must be uploaded via /api/media")
			}
			if _, err := os.Stat(filepath.Join(s.mediaDir, name)); err != nil {
				return errors.New("unknown media file " + name)
			}
		case "link":
			u, err := url.Parse(a.URL)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
				return errors.New("links must be http(s) URLs")
			}
			a.Width, a.Height = 0, 0
		default:
			return errors.New("attachment type must be image, video or link")
		}
	}
	return nil
}
