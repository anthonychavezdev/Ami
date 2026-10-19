package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// The tests drive a real server (routes, middleware, SQLite in a temp dir)
// over HTTP and WebSocket, the way the app does. Each test starts its own.

func TestMain(m *testing.M) {
	pbkdf2Iterations = 1000                // the real count is slow and proves nothing here
	presenceGrace = 300 * time.Millisecond // tests wait for it; keep them quick
	log.SetOutput(io.Discard)              // request and call logging is noise here
	os.Exit(m.Run())
}

const testPassword = "password-test"

type testServer struct {
	t   *testing.T
	s   *server
	url string
}

// newTestServer runs a server on a fresh database; dev seeds the demo content.
func newTestServer(t *testing.T, dev bool) *testServer {
	t.Helper()
	dir := t.TempDir()
	s, err := newServer(t.Context(), config{dbPath: filepath.Join(dir, "test.db"), mediaDir: filepath.Join(dir, "media"), dev: dev, origins: "http://good.example"})
	if err != nil {
		t.Fatal(err)
	}
	s.limiter = newLimiter(1000, time.Minute) // the login budget (10/min) has its own test
	h, err := s.handler("")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(func() {
		srv.Close()
		s.db.Close()
	})
	return &testServer{t: t, s: s, url: srv.URL}
}

// check records a failed expectation without stopping the test, so one run
// reports everything that is off.
func (ts *testServer) check(name string, cond bool, info ...any) {
	ts.t.Helper()
	if !cond {
		ts.t.Errorf("%s: %s", name, fmt.Sprint(info...))
	}
}

// ---- JSON

// J is a decoded JSON object; the accessors return zero values for missing
// or differently typed fields, so checks read like the JSON they inspect.
type J map[string]any

func (j J) num(k string) float64 { v, _ := j[k].(float64); return v }
func (j J) id(k string) string   { return j.str(k) }
func (j J) str(k string) string  { v, _ := j[k].(string); return v }
func (j J) obj(k string) J {
	v, _ := j[k].(map[string]any)
	return J(v)
}
func (j J) list(k string) []J { return objects(j[k]) }
func (j J) has(k string) bool {
	_, ok := j[k]
	return ok
}
func (j J) isNull(k string) bool {
	v, ok := j[k]
	return ok && v == nil
}

// isTrue / isFalse distinguish a real boolean from a missing field.
func (j J) isTrue(k string) bool  { v, ok := j[k].(bool); return ok && v }
func (j J) isFalse(k string) bool { v, ok := j[k].(bool); return ok && !v }

func objects(v any) []J {
	raw, _ := v.([]any)
	out := make([]J, 0, len(raw))
	for _, x := range raw {
		m, _ := x.(map[string]any)
		out = append(out, J(m))
	}
	return out
}

func ids(list []J, k string) []string  { return mapSlice(list, func(j J) string { return j.id(k) }) }
func strs(list []J, k string) []string { return mapSlice(list, func(j J) string { return j.str(k) }) }

func equal[T comparable](a, b []T) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func find(list []J, pred func(J) bool) J {
	for _, j := range list {
		if pred(j) {
			return j
		}
	}
	return nil
}

func all(list []J, pred func(J) bool) bool {
	for _, j := range list {
		if !pred(j) {
			return false
		}
	}
	return true
}

func anyOf(list []J, pred func(J) bool) bool { return find(list, pred) != nil }

// ---- HTTP

type response struct {
	status int
	body   []byte
	header http.Header
}

func (r response) obj() J {
	var j J
	_ = json.Unmarshal(r.body, &j)
	return j
}

func (r response) list() []J {
	var v any
	_ = json.Unmarshal(r.body, &v)
	return objects(v)
}

func (r response) String() string { return fmt.Sprintf("%d %s", r.status, bytes.TrimSpace(r.body)) }

// raw is a request body sent as is, with its own content type.
type raw struct {
	data        []byte
	contentType string
}

// do sends a request; body is JSON-encoded unless it is nil or a raw. Extra
// headers come as name, value pairs.
func (ts *testServer) do(method, path string, body any, token string, headers ...string) response {
	ts.t.Helper()
	var data io.Reader
	contentType := "application/json"
	switch b := body.(type) {
	case nil:
	case raw:
		data, contentType = bytes.NewReader(b.data), b.contentType
	default:
		buf, _ := json.Marshal(b)
		data = bytes.NewReader(buf)
	}
	req, err := http.NewRequest(method, ts.url+path, data)
	if err != nil {
		ts.t.Fatal(err)
	}
	if data != nil {
		req.Header.Set("Content-Type", contentType)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		ts.t.Fatal(err)
	}
	defer res.Body.Close()
	out, _ := io.ReadAll(res.Body)
	return response{status: res.StatusCode, body: out, header: res.Header}
}

func (ts *testServer) get(path, token string) response { return ts.do("GET", path, nil, token) }
func (ts *testServer) post(path string, body any, token string) response {
	if body == nil {
		body = J{}
	}
	return ts.do("POST", path, body, token)
}
func (ts *testServer) put(path string, body any, token string) response {
	return ts.do("PUT", path, body, token)
}
func (ts *testServer) del(path, token string) response { return ts.do("DELETE", path, nil, token) }

// upload posts a file to /api/media.
func (ts *testServer) upload(token string, data []byte, name, contentType string) response {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, _ := w.CreatePart(map[string][]string{
		"Content-Disposition": {fmt.Sprintf(`form-data; name="file"; filename="%s"`, name)},
		"Content-Type":        {contentType},
	})
	part.Write(data)
	w.Close()
	return ts.do("POST", "/api/media", raw{buf.Bytes(), w.FormDataContentType()}, token)
}

// ---- accounts

// account is a signed-in test user.
type account struct {
	token string
	user  J
	id    string
}

func (ts *testServer) register(username string) account {
	ts.t.Helper()
	r := ts.post("/api/register", J{"username": username, "password": testPassword, "display_name": strings.ToUpper(username[:1]) + username[1:]}, "")
	if r.status != 200 {
		ts.t.Fatalf("register %s: %s", username, r)
	}
	return accountOf(r.obj())
}

func (ts *testServer) login(username, password string) account {
	ts.t.Helper()
	r := ts.post("/api/login", J{"username": username, "password": password}, "")
	if r.status != 200 {
		ts.t.Fatalf("login %s: %s", username, r)
	}
	return accountOf(r.obj())
}

func accountOf(j J) account {
	return account{token: j.str("token"), user: j.obj("user"), id: j.obj("user").id("id")}
}

// befriend makes a and b friends (a asks, b accepts).
func (ts *testServer) befriend(a, b account) {
	ts.t.Helper()
	if r := ts.post(fmt.Sprintf("/api/friends/%s/request", b.id), nil, a.token); r.status != 200 {
		ts.t.Fatalf("friend request: %s", r)
	}
	if r := ts.post(fmt.Sprintf("/api/friends/%s/accept", a.id), nil, b.token); r.status != 200 {
		ts.t.Fatalf("accept request: %s", r)
	}
}

// ---- WebSocket

const wsTimeout = 6 * time.Second

type wsClient struct {
	t    *testing.T
	conn *websocket.Conn
}

// ws connects a user's WebSocket. On a refused handshake it returns the HTTP status.
func (ts *testServer) ws(token, origin string) (*wsClient, int) {
	ts.t.Helper()
	header := http.Header{}
	if origin != "" {
		header.Set("Origin", origin)
	}
	url := strings.Replace(ts.url, "http://", "ws://", 1) + "/ws?token=" + token
	conn, res, err := websocket.DefaultDialer.Dial(url, header)
	if err != nil {
		if res != nil {
			return nil, res.StatusCode
		}
		ts.t.Fatalf("ws dial: %v", err)
	}
	w := &wsClient{t: ts.t, conn: conn}
	ts.t.Cleanup(w.close)
	return w, 0
}

// connect opens a WebSocket and consumes its hello.
func (ts *testServer) connect(a account) *wsClient {
	ts.t.Helper()
	w, _ := ts.ws(a.token, "")
	w.expect("hello")
	return w
}

func (w *wsClient) send(v any) {
	w.t.Helper()
	if err := w.conn.WriteJSON(v); err != nil {
		w.t.Fatalf("ws send: %v", err)
	}
}

// recv reads the next event, or nil when nothing arrives in time.
func (w *wsClient) recv(timeout time.Duration) J {
	_ = w.conn.SetReadDeadline(time.Now().Add(timeout))
	var j J
	if err := w.conn.ReadJSON(&j); err != nil {
		return nil
	}
	return j
}

// expect reads until an event of that type arrives; other events are skipped.
func (w *wsClient) expect(typ string) J {
	w.t.Helper()
	e := w.until(func(e J) bool { return e.str("type") == typ }, wsTimeout)
	if e == nil {
		w.t.Fatalf("no %s event", typ)
	}
	return e
}

// until reads until pred accepts an event, or the timeout passes (nil).
func (w *wsClient) until(pred func(J) bool, timeout time.Duration) J {
	end := time.Now().Add(timeout)
	for time.Now().Before(end) {
		e := w.recv(time.Until(end))
		if e == nil {
			return nil
		}
		if pred(e) {
			return e
		}
	}
	return nil
}

func (w *wsClient) close() { w.conn.Close() }

// ---- pictures

// tinyPNG is a 2x2 red picture.
func tinyPNG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for i := range img.Pix {
		img.Pix[i] = 255
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// tinyGIF is a 2x2 two-frame animation.
func tinyGIF() []byte {
	palette := color.Palette{color.RGBA{255, 0, 0, 255}, color.RGBA{0, 0, 255, 255}}
	anim := &gif.GIF{}
	for i := 0; i < 2; i++ {
		frame := image.NewPaletted(image.Rect(0, 0, 2, 2), palette)
		for p := range frame.Pix {
			frame.Pix[p] = uint8(i)
		}
		anim.Image = append(anim.Image, frame)
		anim.Delay = append(anim.Delay, 50)
	}
	var buf bytes.Buffer
	_ = gif.EncodeAll(&buf, anim)
	return buf.Bytes()
}

func path(format string, args ...any) string { return fmt.Sprintf(format, args...) }
