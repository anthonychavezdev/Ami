package main

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// The ICE servers the apps hand to WebRTC for a call. STUN is public and
// free. TURN relays a call when the two sides cannot reach each other, and
// costs bandwidth, so it is the operator's own coturn in use-auth-secret mode:
// this server mints credentials for it that expire (the TURN REST API scheme
// coturn implements: the username carries an expiry, the password is an HMAC
// of it with the shared secret). The apps ask at every join (calls.md).

const turnCredentialTTL = 24 * time.Hour // outlives any call; the app asks again at each join

type iceConfig struct {
	stun     []string // always handed out
	turn     []string // fixed TURN urls; empty means coturn on this host at turnPort
	turnPort int
	secret   string // coturn's static-auth-secret; empty means no TURN
}

func newICEConfig(cfg config) iceConfig {
	return iceConfig{stun: splitList(cfg.stunURLs), turn: splitList(cfg.turnURLs), turnPort: cfg.turnPort, secret: cfg.turnSecret}
}

// splitList reads a comma-separated flag, dropping blanks.
func splitList(s string) []string {
	var out []string
	for _, v := range strings.Split(s, ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// describe says where the TURN credentials point, for the start-up log.
func (c iceConfig) describe() string {
	if len(c.turn) > 0 {
		return strings.Join(c.turn, ", ")
	}
	return fmt.Sprintf("coturn on this host, port %d", c.turnPort)
}

// turnURLs are the configured ones, or coturn next to this server, reached
// at the name the client used for the API (so emulators, phones on the LAN
// and the public hostname all get an address that works for them).
func (c iceConfig) turnURLs(host string) []string {
	if len(c.turn) > 0 {
		return c.turn
	}
	hp := net.JoinHostPort(host, strconv.Itoa(c.turnPort))
	return []string{"turn:" + hp + "?transport=udp", "turn:" + hp + "?transport=tcp"}
}

// requestHost is the name the client reached this server by, without the port.
func requestHost(r *http.Request) string {
	if h, _, err := net.SplitHostPort(r.Host); err == nil {
		return h
	}
	return strings.Trim(r.Host, "[]")
}

// turnCredentials are coturn's use-auth-secret pair: "expiry:user" and the
// base64 HMAC-SHA1 of it under the shared secret. coturn checks the HMAC and
// the expiry itself; no state is kept here.
func turnCredentials(secret, userID string, expiry time.Time) (username, credential string) {
	username = fmt.Sprintf("%d:%s", expiry.Unix(), userID)
	mac := hmac.New(sha1.New, []byte(secret))
	mac.Write([]byte(username))
	return username, base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

type iceServer struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username,omitempty"`
	Credential string   `json:"credential,omitempty"`
}

// iceServers answers GET /api/ice: the STUN servers and, when TURN is set
// up, the TURN servers with credentials for this user good for a day.
func (s *server) iceServers(w http.ResponseWriter, r *http.Request, me *User) error {
	servers := []iceServer{}
	if len(s.ice.stun) > 0 {
		servers = append(servers, iceServer{URLs: s.ice.stun})
	}
	if s.ice.secret != "" {
		username, credential := turnCredentials(s.ice.secret, me.ID, time.Now().Add(turnCredentialTTL))
		servers = append(servers, iceServer{URLs: s.ice.turnURLs(requestHost(r)), Username: username, Credential: credential})
	}
	return ok(w, struct {
		Servers []iceServer `json:"ice_servers"`
		TTL     int         `json:"ttl"` // seconds the TURN credentials stay valid
	}{servers, int(turnCredentialTTL.Seconds())})
}
