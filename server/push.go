package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// Push notifications through Expo's push service, for devices that have no
// WebSocket connection (app closed or suspended). Tokens come from the app;
// they only exist when it was built with an EAS project id (see README).
var expoPushURL = "https://exp.host/--/api/v2/push/send" // a variable so tests can point it at a fake

func validPushToken(t string) bool {
	return len(t) <= 200 && strings.HasSuffix(t, "]") && (strings.HasPrefix(t, "ExponentPushToken[") || strings.HasPrefix(t, "ExpoPushToken["))
}

func (s *server) setPushToken(w http.ResponseWriter, r *http.Request, me *User) error {
	var in struct {
		Token string `json:"token"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	if !validPushToken(in.Token) {
		return badRequest("invalid push token")
	}
	// A token belongs to one device: re-registering moves it to the current user.
	if _, err := s.db.Exec(`INSERT INTO push_tokens(token, user_id, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(token) DO UPDATE SET user_id = excluded.user_id, updated_at = excluded.updated_at`, in.Token, me.ID, nowString()); err != nil {
		return err
	}
	return noContent(w)
}

func (s *server) deletePushToken(w http.ResponseWriter, r *http.Request, me *User) error {
	var in struct {
		Token string `json:"token"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	if _, err := s.db.Exec(`DELETE FROM push_tokens WHERE token = ? AND user_id = ?`, in.Token, me.ID); err != nil {
		return err
	}
	return noContent(w)
}

// notification is what a push says and where tapping it goes.
type notification struct {
	title, body string
	data        map[string]any
}

// messageNotification: a direct message is titled with its sender, a group
// message with the group, naming its author in the body.
func messageNotification(from *User, m Message, groupName string) notification {
	body := m.Body
	if body == "" && len(m.Attachments) > 0 {
		body = map[string]string{"image": "📷 Picture", "video": "🎬 Video", "link": "🔗 Link"}[m.Attachments[0].Type]
	}
	if m.GroupID == nil {
		return notification{title: from.DisplayName, body: body, data: map[string]any{"chat": from.ID}}
	}
	return notification{title: groupName, body: from.DisplayName + ": " + body, data: map[string]any{"group_chat": *m.GroupID}}
}

// pushMessage notifies every registered device of user `to` about a new message.
func (s *server) pushMessage(to string, from *User, m Message, unread int) {
	groupName := ""
	if m.GroupID != nil {
		groupName = s.groupSummary(*m.GroupID).Name
	}
	s.push(to, messageNotification(from, m, groupName), unread)
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

// push sends one notification to every registered device of a user. Best
// effort: failures are logged, dead tokens are forgotten.
func (s *server) push(to string, n notification, badge int) {
	tokens := s.pushTokens(to)
	if len(tokens) == 0 {
		return
	}
	msgs := mapSlice(tokens, func(t string) pushMessage {
		return pushMessage{To: t, Title: n.title, Body: n.body, Data: n.data, Sound: "default", Badge: badge, ChannelID: "messages"}
	})
	for _, dead := range sendPush(msgs) {
		s.db.Exec(`DELETE FROM push_tokens WHERE token = ?`, dead)
	}
}

func (s *server) pushTokens(userID string) []string {
	tokens, _ := queryAll(s.db, func(sc scanner) (string, error) {
		var t string
		return t, sc.Scan(&t)
	}, `SELECT token FROM push_tokens WHERE user_id = ?`, userID)
	return tokens
}

// sendPush posts a batch to Expo and returns the tokens Expo says no longer exist.
func sendPush(msgs []pushMessage) (dead []string) {
	payload, _ := json.Marshal(msgs)
	if verbose {
		shown, _ := json.Marshal(mapSlice(msgs, func(m pushMessage) pushMessage { m.To = "[redacted]"; return m }))
		log.Printf("push → Expo: %s", cut(shown))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, expoPushURL, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("push: %v", err)
		return nil
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
	var body bytes.Buffer
	if err := json.NewDecoder(io.TeeReader(res.Body, &body)).Decode(&out); err != nil || res.StatusCode != http.StatusOK {
		log.Printf("push: status %d", res.StatusCode)
		return nil
	}
	if verbose {
		log.Printf("push ← Expo %d: %s", res.StatusCode, cut(body.Bytes()))
	}
	for i, r := range out.Data {
		if i < len(msgs) && r.Status == "error" && r.Details.Error == "DeviceNotRegistered" {
			dead = append(dead, msgs[i].To)
		}
	}
	return dead
}
