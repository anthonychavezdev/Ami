# Notifications and badges

How a message reaches someone who is not looking at the chat: a local OS
notification while the app runs, a push through Expo when it does not, and the
unread badges in between. Code: [`server/push.go`](../server/push.go); in the app
[`notifications.ts`](../mobile/src/lib/notifications.ts) and its
[`.web`](../mobile/src/lib/notifications.web.ts) twin, [`unread.ts`](../mobile/src/lib/unread.ts),
and the tap handling in [`app/_layout.tsx`](../mobile/src/app/_layout.tsx).

## Three cases

| The recipient is... | What happens | Where |
| --- | --- | --- |
| looking at that chat (app in the foreground) | The message appears and is marked read; no notification. | `unread.ts` (`reactionTo` → `read`) |
| connected but elsewhere (another screen, app in the background, another tab) | The `message` event arrives over the WebSocket; the app shows a **local** notification and bumps the badge. | `unread.ts` → `notify` |
| not connected (app closed, suspended, no network) | The server sends a **push** through Expo to every device that registered a token. | `push.go` (`deliverMessage` → `pushMessage`) |

Only chat messages notify. Posts, comments, reactions and friend requests
update screens live but never raise a notification.

## PUT /api/push-token

Body `{"token": "ExponentPushToken[...]"}` (or the `ExpoPushToken[` prefix),
at most 200 characters, 400 otherwise. The row is upserted on the token, so a
token moves to whoever is signed in on that device. 204.

## DELETE /api/push-token

Body `{"token": "..."}`; deletes the row if it belongs to the caller. The app
calls it on logout. 204.

## Pushing from the server

`deliverMessage` in `messages.go` tries the WebSocket first; when `sendTo`
reports that the user has no live connection, `pushMessage` runs in the
background:

- The notification is titled with the sender's name for a direct message, or
  with the group's name and `Sender: text` as the body for a group message.
  A message without text says 📷 Picture, 🎬 Video or 🔗 Link.
- `data` carries `{chat: senderId}` or `{group_chat: groupId}` so a tap opens
  the right chat.
- Every token of the user goes in one batch to `https://exp.host/--/api/v2/push/send`
  with `sound: "default"`, `badge` set to the user's unread total and
  `channelId: "messages"` (the Android channel the app creates).
- Expo's answer is read per ticket: a `DeviceNotRegistered` error deletes that
  token. Any other failure is logged and ignored; push is best effort.
- With `-verbose`, the batch (tokens redacted) and Expo's reply are logged.

The test suite points `expoPushURL` at a fake Expo to check all of this
([testing.md](testing.md)).

## In the app

### Devices (`notifications.ts`)

- `setNotificationHandler` makes notifications show as banners with sound and
  badge even while the app is in the foreground (the unread store decides when
  to post one).
- `setupNotifications()` runs on sign-in: creates the Android channel
  `messages` (high importance), asks for permission, and, when the app was built
  with an EAS project id (`extra.eas.projectId` in `app.json`) **and** runs on a
  physical device, fetches the Expo push token and registers it with
  `PUT /api/push-token`. Without a project id the app never registers a token
  and push simply does not happen; everything else still works.
- `notify(title, body, data)` schedules an immediate local notification (on the
  `messages` channel on Android).
- `onNotificationTap(handler)` subscribes to taps and also handles the
  notification that launched the app (once). The root layout's handler pushes
  `/chat/<id>` or `/chat/group/<id>` from the notification's `data`.
- `setBadge(n)` sets the app icon badge; `teardownNotifications()` on logout
  forgets the push token server-side and clears the badge.

### Browser (`notifications.web.ts`)

The Notification API. Permission is requested on the login and register
buttons, because browsers only allow the prompt from a click. Notifications are
tagged per chat so a chat's newer message replaces its older one, clicking one
focuses the window and opens the chat, and the "badge" is a `(3) Ami` prefix on
the tab title, re-applied whenever the router rewrites the title. There is no
push in the browser: the socket keeps delivering while the tab is open, even in
the background.

### The unread store (`unread.ts`)

Holds the total for the Messages tab badge and mirrors it to the OS badge. The
server is the source of truth: `hello`, `messages.read` and `message.deleted`
carry the total, and a `message` event for a recipient carries it too. For each
incoming `message` the store decides: ignore (it is yours), mark read (that chat
is open and the app is visible) or notify (update the count and post a
notification with the sender's name, the group's name when there is one, and a
text or attachment preview).

## Setting push up for a real deployment

Push needs an EAS project and platform credentials: `eas init` in `mobile/`
adds the project id to `app.json`, `eas credentials` uploads FCM (Android) and
APNs (iOS) keys. Builds without them work, just without push
([deployment.md](deployment.md#push-notifications)).
