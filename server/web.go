package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// webApp serves the exported browser build (`build web` -> dist/web). Files
// that exist are served as is; any other path gets index.html so the app's
// client-side router can handle it. Expo hashes bundle names, so those are
// cacheable forever; index.html keeps the no-store default.
func webApp(dir string) http.Handler {
	files := http.FileServer(http.Dir(dir))
	index := filepath.Join(dir, "index.html")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := filepath.Clean("/" + r.URL.Path)
		if strings.HasPrefix(p, "/api/") {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		if st, err := os.Stat(filepath.Join(dir, filepath.FromSlash(p))); err == nil && !st.IsDir() {
			if strings.HasPrefix(p, "/_expo/") || strings.HasPrefix(p, "/assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			files.ServeHTTP(w, r)
			return
		}
		h := w.Header()
		// The bundle is the only script; react-native-web sets inline styles.
		h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; media-src 'self' blob:; font-src 'self' data:; connect-src 'self' ws://"+r.Host+" wss://"+r.Host+"; worker-src 'self' blob:; frame-ancestors 'none'; object-src 'none'; base-uri 'self'; form-action 'self'")
		h.Set("Permissions-Policy", "camera=(self), microphone=(self), display-capture=(self), geolocation=()")
		http.ServeFile(w, r, index)
	})
}
