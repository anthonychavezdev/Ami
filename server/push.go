package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"
)

// Push notifications through Expo's push service, for devices that have no
// WebSocket connection (app closed or suspended). Tokens come from the app;
// they only exist when it was built with an EAS project id (see README).
const expoPushURL = "https://exp.host/--/api/v2/push/send"

func validPushToken(t string) bool {
	return len(t) <= 200 && strings.HasSuffix(t, "]") && (strings.HasPrefix(t, "ExponentPushToken[") || strings.HasPrefix(t, "ExpoPushToken["))
}

func (s *server) setPushToken(w http.ResponseWriter, r *http.Request, me *User) {
	var in struct {
		Token string `json:"token"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if !validPushToken(in.Token) {
		writeError(w, http.StatusBadRequest, "invalid push token")
		return
	}
	// A token belongs to one device: re-registering moves it to the current user.
	if _, err := s.db.Exec(`INSERT INTO push_tokens(token, user_id, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(token) DO UPDATE SET user_id = excluded.user_id, updated_at = excluded.updated_at`, in.Token, me.ID, nowString()); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) deletePushToken(w http.ResponseWriter, r *http.Request, me *User) {
	var in struct {
		Token string `json:"token"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if _, err := s.db.Exec(`DELETE FROM push_tokens WHERE token = ? AND user_id = ?`, in.Token, me.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type pushMessage struct {
	To        string         `json:"to"`
	Title     string         `json:"title"`
	Body      string         `json:"body"`
	Data      map[string]any `json:"data,omitempty"`
	Sound     string         `json:"sound,omitempty"`
	Badge     int            `json:"badge,omitempty"`
	ChannelID string         `json:"channelId,omitempty"`
}

// pushMessage notifies every registered device of user `to` about a new
// message. Best effort: failures are logged, dead tokens are forgotten.
func (s *server) pushMessage(to int64, from *User, m Message, unread int) {
	rows, err := s.db.Query(`SELECT token FROM push_tokens WHERE user_id = ?`, to)
	if err != nil {
		return
	}
	var tokens []string
	for rows.Next() {
		var t string
		if rows.Scan(&t) == nil {
			tokens = append(tokens, t)
		}
	}
	rows.Close()
	if len(tokens) == 0 {
		return
	}
	body := m.Body
	if body == "" && len(m.Attachments) > 0 {
		body = map[string]string{"image": "📷 Picture", "video": "🎬 Video", "link": "🔗 Link"}[m.Attachments[0].Type]
	}
	msgs := make([]pushMessage, 0, len(tokens))
	for _, t := range tokens {
		msgs = append(msgs, pushMessage{To: t, Title: from.DisplayName, Body: body, Data: map[string]any{"chat": from.ID}, Sound: "default", Badge: unread, ChannelID: "messages"})
	}
	payload, _ := json.Marshal(msgs)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, expoPushURL, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("push: %v", err)
		return
	}
	defer res.Body.Close()
	var out struct {
		Data []struct {
			Status  string `json:"status"`
			Details struct {
				Error string `json:"error"`
			} `json:"details"`
		} `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil || res.StatusCode != http.StatusOK {
		log.Printf("push: status %d", res.StatusCode)
		return
	}
	for i, r := range out.Data {
		if i < len(tokens) && r.Status == "error" && r.Details.Error == "DeviceNotRegistered" {
			s.db.Exec(`DELETE FROM push_tokens WHERE token = ?`, tokens[i])
		}
	}
}
