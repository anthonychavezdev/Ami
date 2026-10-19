package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

// -verbose shows everything the server receives and sends: each request with
// its headers and body, each response with its status, headers and body, and
// every WebSocket message in and out. Requests are numbered so their two
// lines pair up among concurrent ones. Passwords and tokens are redacted and
// bodies are cut at logLimit, so the log stays safe to keep and to share.
var verbose bool

const logLimit = 4 << 10

var requestSeq atomic.Int64

// requestInfo is what a request's response line says besides the response:
// its number, and the user once auth has resolved the session.
type requestInfo struct {
	n    int64
	user string
}

type requestInfoKey struct{}

// noteUser records whose session a request carries, for its log line.
func noteUser(r *http.Request, userID string) {
	if info, ok := r.Context().Value(requestInfoKey{}).(*requestInfo); ok {
		info.user = userID
	}
}

// secretFields names, per endpoint, the body fields (in the request or the
// response) that hold credentials; they are shown as [redacted].
var secretFields = map[string][]string{
	"POST /api/login":      {"password", "token"},
	"POST /api/register":   {"password", "token"},
	"PUT /api/me/password": {"current", "new"},
	"PUT /api/push-token":  {"token"},
}

// logTraffic takes the place of logRequests under -verbose.
func logTraffic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		info := &requestInfo{n: requestSeq.Add(1)}
		r = r.WithContext(context.WithValue(r.Context(), requestInfoKey{}, info))
		secrets := secretFields[r.Method+" "+r.URL.Path]
		log.Printf("#%d → %s %s from %s\n%s%s", info.n, r.Method, requestURI(r), r.RemoteAddr,
			headerLines(r.Header), bodyLines(r.Header.Get("Content-Type"), requestBody(r), r.ContentLength, secrets))
		cw := &captureWriter{statusWriter: statusWriter{ResponseWriter: w, status: http.StatusOK}}
		next.ServeHTTP(cw, r)
		who := ""
		if info.user != "" {
			who = " user " + info.user
		}
		log.Printf("#%d ← %d in %s%s\n%s%s", info.n, cw.status, time.Since(start).Round(time.Millisecond), who,
			headerLines(cw.Header()), bodyLines(cw.Header().Get("Content-Type"), cw.body.Bytes(), cw.size, secrets))
	})
}

// captureWriter keeps the start of the response body for the log.
type captureWriter struct {
	statusWriter
	body bytes.Buffer
	size int64
}

func (w *captureWriter) Write(p []byte) (int, error) {
	w.size += int64(len(p))
	if room := logLimit + 1 - w.body.Len(); room > 0 {
		w.body.Write(p[:min(room, len(p))])
	}
	return w.statusWriter.Write(p)
}

// requestURI is the path and query, with a ?token= (the WebSocket's) hidden.
func requestURI(r *http.Request) string {
	if q := r.URL.Query(); q.Has("token") {
		q.Set("token", "redacted")
		return r.URL.Path + "?" + q.Encode()
	}
	return r.URL.RequestURI()
}

// headerLines lists headers one per line, the Authorization value hidden.
func headerLines(h http.Header) string {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		v := strings.Join(h[k], ", ")
		if k == "Authorization" {
			scheme, _, _ := strings.Cut(v, " ")
			v = scheme + " [redacted]"
		}
		fmt.Fprintf(&b, "    %s: %s\n", k, v)
	}
	return b.String()
}

// textual says whether a body is worth printing as it is.
func textual(contentType string) bool {
	return strings.HasPrefix(contentType, "application/json") || strings.HasPrefix(contentType, "text/") ||
		strings.HasPrefix(contentType, "application/x-www-form-urlencoded")
}

// requestBody peeks the start of a text body for the log and hands the whole
// body on to the handler untouched. Uploads are left alone.
func requestBody(r *http.Request) []byte {
	if r.Body == nil || r.ContentLength == 0 || !textual(r.Header.Get("Content-Type")) {
		return nil
	}
	head := make([]byte, logLimit+1)
	n, _ := io.ReadFull(r.Body, head)
	r.Body = readCloser{io.MultiReader(bytes.NewReader(head[:n]), r.Body), r.Body}
	return head[:n]
}

type readCloser struct {
	io.Reader
	io.Closer
}

// bodyLines renders a body for the log: JSON and text as they are, with the
// secret fields redacted, anything else by type and size; long ones are cut.
func bodyLines(contentType string, body []byte, size int64, secrets []string) string {
	if !textual(contentType) {
		if size > 0 {
			return fmt.Sprintf("    (%s, %d bytes)\n", contentType, size)
		}
		return ""
	}
	if len(body) == 0 {
		return ""
	}
	if len(secrets) > 0 {
		var ok bool
		if body, ok = redacted(body, secrets); !ok {
			return "    (not shown: has credentials and could not be redacted)\n"
		}
	}
	cut := ""
	if len(body) > logLimit {
		body, cut = body[:logLimit], " …"
	}
	return "    " + strings.TrimRight(string(body), "\n") + cut + "\n"
}

// redacted replaces the named top-level fields of a JSON object.
func redacted(body []byte, keys []string) ([]byte, bool) {
	var obj map[string]json.RawMessage
	if json.Unmarshal(body, &obj) != nil {
		return nil, false
	}
	for _, k := range keys {
		if _, ok := obj[k]; ok {
			obj[k] = json.RawMessage(`"[redacted]"`)
		}
	}
	out, err := json.Marshal(obj)
	return out, err == nil
}

// cut is a WebSocket or push payload for the log, shortened past logLimit.
func cut(data []byte) string {
	if len(data) > logLimit {
		return string(data[:logLimit]) + " …"
	}
	return string(data)
}
