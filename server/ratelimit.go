package main

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// limiter is a fixed-window request limiter keyed by client IP, used to slow
// down credential stuffing on the auth endpoints.
type limiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	hits   map[string]*hitWindow
}

type hitWindow struct {
	start time.Time
	n     int
}

func newLimiter(max int, window time.Duration) *limiter {
	return &limiter{max: max, window: window, hits: map[string]*hitWindow{}}
}

func (l *limiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	t := time.Now()
	if len(l.hits) > 4096 {
		for k, w := range l.hits {
			if t.Sub(w.start) > l.window {
				delete(l.hits, k)
			}
		}
	}
	w := l.hits[key]
	if w == nil || t.Sub(w.start) > l.window {
		l.hits[key] = &hitWindow{start: t, n: 1}
		return true
	}
	w.n++
	return w.n <= l.max
}

// clientIP is the peer address, or, when the server is told it sits behind a
// trusted reverse proxy, the last X-Forwarded-For hop: the one that proxy
// appended, which a client cannot choose by sending the header itself.
func (s *server) clientIP(r *http.Request) string {
	if s.trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			hops := strings.Split(xff, ",")
			return strings.TrimSpace(hops[len(hops)-1])
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// throttle answers 429 once a client exceeds the limiter.
func (s *server) throttle(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.limiter.allow(s.clientIP(r)) {
			w.Header().Set("Retry-After", "60")
			writeError(w, http.StatusTooManyRequests, "too many attempts, try again in a minute")
			return
		}
		h(w, r)
	}
}
