package main

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// The verbose log shows the whole exchange with credentials hidden, and the
// handler still gets the whole body.
func TestVerboseLog(t *testing.T) {
	var out bytes.Buffer
	log.SetOutput(&out)
	defer log.SetOutput(os.Stderr)
	body := `{"username":"ana","password":"amigo-demo"}`
	var got string
	h := logTraffic(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = string(b)
		noteUser(r, "u7")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"token":"s3cret-session","user":{"id":7}}`))
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/login?token=qtok123", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer s3cret-session")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if got != body {
		t.Fatalf("handler got body %q", got)
	}
	logged := out.String()
	for _, want := range []string{"→ POST /api/login?token=redacted", `"username":"ana"`, `"password":"[redacted]"`, "Bearer [redacted]",
		"← 200 in", "user u7", `"token":"[redacted]"`, `"user":{"id":7}`} {
		if !strings.Contains(logged, want) {
			t.Errorf("log lacks %q:\n%s", want, logged)
		}
	}
	for _, leak := range []string{"amigo-demo", "s3cret-session", "qtok123"} {
		if strings.Contains(logged, leak) {
			t.Errorf("log leaks %q:\n%s", leak, logged)
		}
	}
}

// A body too long to redact safely is not shown at all, but still reaches the handler.
func TestVerboseLogLongCredentials(t *testing.T) {
	var out bytes.Buffer
	log.SetOutput(&out)
	defer log.SetOutput(os.Stderr)
	body := `{"password":"amigo-demo","pad":"` + strings.Repeat("x", logLimit) + `"}`
	var got int
	h := logTraffic(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = len(b)
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if got != len(body) {
		t.Fatalf("handler got %d of %d bytes", got, len(body))
	}
	if logged := out.String(); strings.Contains(logged, "amigo-demo") || !strings.Contains(logged, "not shown") {
		t.Errorf("long credentials leaked or not explained:\n%s", logged)
	}
}
