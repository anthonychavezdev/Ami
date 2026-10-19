package main

import (
	"encoding/json"
	"image"
	"math"
	"os"
	"path/filepath"
	"strings"
)

// A profile has two pictures: the round profile picture and the wide cover.
// Each is an upload under /media plus a Crop, the window of it that is shown.

// Crop is a window on a picture: X and Y are the top-left corner as fractions
// of the picture's width and height, W the window's width as a fraction of
// the picture's width, and Aspect the picture's width/height ratio. The
// window's own shape is fixed per picture (1 for the profile picture, which
// is shown in a circle; coverRatio for the cover), so its height as a
// fraction of the picture's height is W*Aspect/ratio. Stored as JSON text.
type Crop struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	W      float64 `json:"w"`
	Aspect float64 `json:"aspect"`
}

// picture names one of the two: its columns and the shape of its window.
type picture struct {
	urlColumn, cropColumn string
	ratio                 float64 // the window's width/height
}

const coverRatio = 3

var (
	avatarPicture = picture{"avatar_url", "avatar_crop", 1}
	coverPicture  = picture{"cover_url", "cover_crop", coverRatio}
)

var pictureExts = map[string]bool{".jpg": true, ".png": true, ".gif": true, ".webp": true}

func (c *Crop) Scan(src any) error {
	*c = Crop{}
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
func (c Crop) rounded() Crop {
	r := func(v float64) float64 { return math.Round(v*10000) / 10000 }
	return Crop{X: r(c.X), Y: r(c.Y), W: r(c.W), Aspect: r(c.Aspect)}
}

// valid says whether a window of the given ratio fits inside the picture.
func (c Crop) valid(ratio float64) bool {
	const eps = 0.001
	return c.W > 0 && c.W <= 1+eps && c.Aspect > 0.05 && c.Aspect < 20 && c.X >= -eps && c.Y >= -eps &&
		c.X+c.W <= 1+eps && c.Y+c.W*c.Aspect/ratio <= 1+eps
}

// defaultCrop is the largest centred window of the given ratio on a picture
// of the given aspect ratio; an unknown aspect (0) is taken to be the window's.
func defaultCrop(aspect, ratio float64) Crop {
	if aspect <= 0 {
		aspect = ratio
	}
	w := math.Min(1, ratio/aspect)
	return Crop{X: (1 - w) / 2, Y: (1 - w*aspect/ratio) / 2, W: w, Aspect: aspect}.rounded()
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

// picturePatch adds one picture and its crop to a profile change. The picture
// decides its aspect ratio and a new picture without a crop gets the centred
// window, so a picture always has a usable crop; no picture means no crop.
func (s *server) picturePatch(p columnPatch, userID string, pic picture, url *string, crop *Crop) (columnPatch, error) {
	current, changed := "", false
	if url != nil {
		a, err := s.validPicture(*url)
		if err != nil {
			return p, err
		}
		if a == "" {
			return p.with(pic.urlColumn, "").with(pic.cropColumn, ""), nil
		}
		current, changed = a, true
		p = p.with(pic.urlColumn, a)
	}
	if !changed && crop == nil {
		return p, nil
	}
	if !changed {
		var err error
		if current, err = s.currentPicture(userID, pic.urlColumn); err != nil {
			return p, err
		}
	}
	c, err := s.cropFor(current, crop, pic.ratio)
	if err != nil {
		return p, err
	}
	return p.with(pic.cropColumn, jsonString(c)), nil
}

// validPicture accepts "" (no picture) or an uploaded picture under /media.
func (s *server) validPicture(raw string) (string, error) {
	a := strings.TrimSpace(raw)
	if a == "" {
		return "", nil
	}
	name, ok := uploadName(a)
	if !ok || !pictureExts[strings.ToLower(filepath.Ext(name))] {
		return "", badRequest("profile pictures must be pictures uploaded via /api/media")
	}
	if !s.uploadExists(name) {
		return "", badRequest("unknown media file " + name)
	}
	return a, nil
}

func (s *server) currentPicture(userID, column string) (string, error) {
	var url string
	if err := s.db.QueryRow(`SELECT `+column+` FROM users WHERE id = ?`, userID).Scan(&url); err != nil || url == "" {
		return "", badRequest("no picture to position")
	}
	return url, nil
}

// cropFor is the crop to store: the requested one on the picture's real
// aspect ratio, or the centred window when none was requested.
func (s *server) cropFor(url string, requested *Crop, ratio float64) (Crop, error) {
	aspect := s.pictureAspect(url)
	if requested == nil {
		return defaultCrop(aspect, ratio), nil
	}
	crop := *requested
	if aspect > 0 {
		crop.Aspect = aspect
	}
	if !crop.valid(ratio) {
		return crop, badRequest("the crop must be a window inside the picture: x, y, w in 0-1")
	}
	return crop.rounded(), nil
}
