package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// fakeExpo stands in for Expo's push service: it records every batch and
// reports one token as no longer registered.
func fakeExpo(t *testing.T, deadToken string) (<-chan []pushMessage, func()) {
	batches := make(chan []pushMessage, 8)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var msgs []pushMessage
		_ = json.NewDecoder(r.Body).Decode(&msgs)
		batches <- msgs
		type ticket struct {
			Status  string         `json:"status"`
			Details map[string]any `json:"details,omitempty"`
		}
		var out struct {
			Data []ticket `json:"data"`
		}
		for _, m := range msgs {
			if m.To == deadToken {
				out.Data = append(out.Data, ticket{Status: "error", Details: map[string]any{"error": "DeviceNotRegistered"}})
			} else {
				out.Data = append(out.Data, ticket{Status: "ok"})
			}
		}
		_ = json.NewEncoder(w).Encode(out)
	}))
	old := expoPushURL
	expoPushURL = srv.URL
	return batches, func() {
		expoPushURL = old
		srv.Close()
	}
}

func TestPush(t *testing.T) {
	ts := newTestServer(t, false)
	batches, stop := fakeExpo(t, "ExponentPushToken[dead]")
	defer stop()
	t1, t2 := ts.register("tom"), ts.register("tia")
	ts.befriend(t1, t2)
	ts.put("/api/push-token", J{"token": "ExponentPushToken[dead]"}, t2.token)
	ts.put("/api/push-token", J{"token": "ExponentPushToken[live]"}, t2.token)

	// t2 has no connection, so a message to them goes out as a push with their badge count.
	ts.post(path("/api/messages/%s", t2.id), J{"body": "¿Estás ahí?"}, t1.token)
	select {
	case msgs := <-batches:
		ts.check("a message to an offline user is pushed to every device", len(msgs) == 2 && msgs[0].Title == "Tom" && msgs[0].Body == "¿Estás ahí?" && msgs[0].Badge == 1 && msgs[0].Data["chat"] == t1.id, msgs)
	case <-time.After(wsTimeout):
		t.Error("no push was sent")
	}
	deadline := time.Now().Add(wsTimeout)
	for time.Now().Before(deadline) && len(ts.s.pushTokens(t2.id)) != 1 {
		time.Sleep(20 * time.Millisecond)
	}
	ts.check("tokens Expo no longer knows are forgotten, live ones kept", equal(ts.s.pushTokens(t2.id), []string{"ExponentPushToken[live]"}), ts.s.pushTokens(t2.id))

	// Online users are not pushed.
	ts.connect(t2)
	ts.post(path("/api/messages/%s", t2.id), J{"body": "otra"}, t1.token)
	select {
	case msgs := <-batches:
		t.Errorf("a connected user was pushed: %v", msgs)
	case <-time.After(300 * time.Millisecond):
	}
}
