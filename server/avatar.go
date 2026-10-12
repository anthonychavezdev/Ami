package main

import (
	"encoding/json"
	"image"
	"math"
	"os"
	"path/filepath"
	"strings"
)

// Profile pictures: an upload under /media plus the square of it shown in the
// circle.

var avatarExts = map[string]bool{".jpg": true, ".png": true, ".gif": true, ".webp": true}

// AvatarCrop is a square window on the profile picture: X and Y are the
// top-left corner as fractions of the width and height, W the side as a
// fraction of the width, and Aspect the picture's width/height ratio (the
// side as a fraction of the height is W*Aspect). Stored as JSON text.
type AvatarCrop struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	W      float64 `json:"w"`
	Aspect float64 `json:"aspect"`
}

func (c *AvatarCrop) Scan(src any) error {
	*c = AvatarCrop{}
	var raw string
	switch v := src.(type) {
	case string:
		raw = v
	case []byte:
		raw = string(v)
	}
	if raw == "" {
		return nil
	}
	return json.Unmarshal([]byte(raw), c)
}

// rounded trims floating-point noise (e.g. 2.8e-17) before storing.
func (c AvatarCrop) rounded() AvatarCrop {
	r := func(v float64) float64 { return math.Round(v*10000) / 10000 }
	return AvatarCrop{X: r(c.X), Y: r(c.Y), W: r(c.W), Aspect: r(c.Aspect)}
}

func (c AvatarCrop) valid() bool {
	const eps = 0.001
	return c.W > 0 && c.W <= 1+eps && c.Aspect > 0.05 && c.Aspect < 20 && c.X >= -eps && c.Y >= -eps && c.X+c.W <= 1+eps && c.Y+c.W*c.Aspect <= 1+eps
}

// defaultCrop is the largest centred square of a picture.
func defaultCrop(aspect float64) AvatarCrop {
	if aspect <= 0 {
		aspect = 1
	}
	w := math.Min(1, 1/aspect)
	return AvatarCrop{X: (1 - w) / 2, Y: (1 - w*aspect) / 2, W: w, Aspect: aspect}.rounded()
}

// pictureAspect is width/height of an uploaded picture, or 0 when it cannot
// be decoded (WebP has no decoder in the standard library).
func (s *server) pictureAspect(url string) float64 {
	f, err := os.Open(filepath.Join(s.mediaDir, strings.TrimPrefix(url, "/media/")))
	if err != nil {
		return 0
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil || cfg.Height == 0 {
		return 0
	}
	return float64(cfg.Width) / float64(cfg.Height)
}

// avatarPatch adds the picture and its crop to a profile change. The picture
// decides its aspect ratio and a new picture without a crop gets the centred
// square, so every avatar always has a usable crop; no picture means no crop.
func (s *server) avatarPatch(p columnPatch, userID int64, in profileInput) (columnPatch, error) {
	url, changed := "", false
	if in.AvatarURL != nil {
		a, err := s.validAvatar(*in.AvatarURL)
		if err != nil {
			return p, err
		}
		if a == "" {
			return p.with("avatar_url", "").with("avatar_crop", ""), nil
		}
		url, changed = a, true
		p = p.with("avatar_url", a)
	}
	if !changed && in.AvatarCrop == nil {
		return p, nil
	}
	if !changed {
		var err error
		if url, err = s.currentAvatar(userID); err != nil {
			return p, err
		}
	}
	crop, err := s.cropFor(url, in.AvatarCrop)
	if err != nil {
		return p, err
	}
	return p.with("avatar_crop", jsonString(crop)), nil
}

// validAvatar accepts "" (no picture) or an uploaded picture under /media.
func (s *server) validAvatar(raw string) (string, error) {
	a := strings.TrimSpace(raw)
	if a == "" {
		return "", nil
	}
	name, ok := uploadName(a)
	if !ok || !avatarExts[strings.ToLower(filepath.Ext(name))] {
		return "", badRequest("profile pictures must be pictures uploaded via /api/media")
	}
	if !s.uploadExists(name) {
		return "", badRequest("unknown media file " + name)
	}
	return a, nil
}

func (s *server) currentAvatar(userID int64) (string, error) {
	var url string
	if err := s.db.QueryRow(`SELECT avatar_url FROM users WHERE id = ?`, userID).Scan(&url); err != nil || url == "" {
		return "", badRequest("no profile picture to position")
	}
	return url, nil
}

// cropFor is the crop to store: the requested one on the picture's real
// aspect ratio, or the centred square when none was requested.
func (s *server) cropFor(url string, requested *AvatarCrop) (AvatarCrop, error) {
	aspect := s.pictureAspect(url)
	if requested == nil {
		return defaultCrop(aspect), nil
	}
	crop := *requested
	if aspect > 0 {
		crop.Aspect = aspect
	}
	if !crop.valid() {
		return crop, badRequest("avatar_crop must be a square inside the picture: x, y, w in 0-1")
	}
	return crop.rounded(), nil
}
