# The WebSocket and the hub

One WebSocket per app instance carries every realtime event, presence, and the
call signaling. Code: [`server/hub.go`](../server/hub.go), [`server/ws.go`](../server/ws.go),
[`server/events.go`](../server/events.go), [`server/presence.go`](../server/presence.go);
in the app [`ws.ts`](../mobile/src/lib/ws.ts), [`events.ts`](../mobile/src/lib/events.ts),
[`realtime.ts`](../mobile/src/lib/realtime.ts), [`presence.ts`](../mobile/src/lib/presence.ts).

## Connecting

`GET /ws` with the session token in the handshake's subprotocols,
`Sec-WebSocket-Protocol: ami, <session token>`: browsers can set no other
header on a WebSocket handshake, and a token in the URL would be written to
every proxy and browser log on the way. The server agrees to `ami` alone
(`wsToken`, `Upgrader.Subprotocols`). `handleWS`:

1. Resolves the session from that token; none or a bad one is `401`. The
   `must_change_password` restriction does not apply to the socket itself.
2. Checks the browser origin like CORS does (`originAllowed`); a refused origin
   is `403` from the upgrader. Native apps send no `Origin`.
3. Records whether the user chose to appear offline (`setHidden`) *before*
   adding the connection, so an invisible user is never announced.
4. Upgrades (gorilla/websocket, 4 KB buffers), registers the connection in the
   hub (`add`), starts the write loop, and sends `hello`.
5. Reads frames until the connection drops; each is handed to
   `handleWSMessage`. More than 300 messages in 10 s closes the connection.
   An account may hold 16 connections at once (`maxSocketsPerUser`; a dead
   one is gone within the pong wait), and the handshake itself counts
   against the per-address API budget (`-api-rate`), so a flood of
   handshakes is 429 before any session lookup.
6. On exit: `remove` from the hub, a timer to free any call seats after the
   presence grace, a close frame, and the socket is closed.

Transport settings in `ws.go`: messages up to 64 KB, a ping every 30 s, a peer
that does not answer within 90 s is dropped, writes time out after 10 s.

## The hub

```go
type hub struct {
    users    map[int64]map[*client]struct{} // every live connection, per user
    leaving  map[int64]*time.Timer          // users inside the presence grace period
    hidden   map[int64]bool                 // users who appear offline
    decorate func(v any) any                // fills presence into outgoing events
}
```

Each `client` has a buffered `send` channel (64 frames) and one `writeLoop`
goroutine, because gorilla allows a single writer per connection. `queue` drops
a frame when the channel is full rather than blocking a handler on a slow
client; a client that falls that far behind reconnects and refetches.

Sending:

| Function | Reaches |
| --- | --- |
| `sendTo(userID, v)` | every connection of one user; returns whether there was any (used to decide on push) |
| `sendToMany(ids, v)` | every connection of several users |
| `sendAll(v)` | every connection (presence changes) |
| `sendToClient(c, v)` | one connection (`call.joined`, `error`) |

All of them marshal once through `decorate` (presence) and copy the bytes to
each queue. With `-verbose` every delivery is logged with its recipients.

## Presence

"Online" means having a live connection. To avoid flicker on reconnects, a user
whose last connection drops stays present for `presenceGrace` (5 s) and only
then is announced offline; coming back within the grace cancels the announcement.

- `add` announces `presence {user_id, online: true}` to everyone when a user's
  first connection arrives (not for a second device, not for a return within
  the grace), unless the user is hidden.
- `remove` starts the grace timer when the last connection goes; the timer
  announces `online: false` unless the user came back or is hidden.
- `setHidden` (from `PUT /api/me/presence`) announces an offline or online
  event when the user is connected and the flag actually changed.
- `present(id)` is what others see: connected or within the grace, and not
  hidden. `online(id)` is the raw "can I deliver right now" and is what
  `broadcastPost` and push decisions use.
- `onlineIDs()` lists everyone present for `hello`.

Every `User` in every HTTP response and event carries `online`, filled in by
reflection ([server.md](server.md#presence-in-every-response)). The app keeps a
map of ids to online flags ([`presence.ts`](../mobile/src/lib/presence.ts)),
seeded from user objects as they render, replaced wholesale by `hello`'s list,
and updated by `presence` events; `useOnline(id)` drives the dot on every avatar.

## Events

Every event is a struct in `events.go` and a member of the `ServerEvent` union in
`events.ts`, field for field. Changing one means changing both; `socket.on` is
typed from the union so a handler only sees the fields its event has.

### Server to client

| `type` | Fields | Sent when, and to whom |
| --- | --- | --- |
| `hello` | `user: Me-like User`, `unread`, `online: [ids]` | Right after connecting, to the new connection. |
| `presence` | `user_id`, `online` | Someone appears or disappears; to everyone connected. |
| `pong` | | Answer to a client `ping`. |
| `error` | `error` | A client message was refused (calls); to that connection. |
| `message` | `message`, `sender`, `group?`, `unread?` | A new chat message; to the sender's devices without `unread`, to the other members with their total. |
| `message.updated` | `message` | An edit; to everyone in the chat. |
| `message.deleted` | `id`, `sender_id`, `recipient_id`, `group_id`, `unread` | A deletion; to everyone in the chat, each with their own total. |
| `messages.read` | `peer_id?` or `group_id?`, `unread` | You read a chat, left a group, or a group was deleted; to your own devices. |
| `group.added` | `group`, `by` | An admin added you to a group. |
| `friend.changed` | `user`, `status` | Any friendship change; to both people, each with their own status. |
| `post.created`, `post.updated` | `post` | A new post, or an edit, reaction or comment-count change; to online members of the post's audience, rendered per viewer. |
| `post.deleted` | `id`, `group_id` | To the post's audience. |
| `comment.created`, `comment.updated` | `comment` | To the post's audience. |
| `comment.deleted` | `post_id`, `id` | To the post's audience. |
| `call.ring` | `room`, `video`, `from_user`, `group?` | A call you may join started; to the friend, or every group member. |
| `call.joined` | `room`, `video`, `group?`, `participants` | Answer to your `call.join`; to that connection only. |
| `call.peer_joined` | `room`, `user` | Someone took a seat; to everyone who could join. |
| `call.peer_left` | `room`, `user_id` | Someone left; same audience. |
| `call.declined` | `room`, `user_id`, `reason` | Someone rung is not coming; to the participants. |
| `call.ended` | `room` | The room emptied; to everyone who could join. |
| `call.unavailable` | `room`, `user_id` | A one-to-one call reached nobody; to the caller. |
| `call.offer`, `call.answer`, `call.ice`, `call.streams` | `room`, `from`, `from_user` (offers), `sdp` / `candidate` / `camera`+`screen` | Signaling relayed from another participant. |

The app adds two pseudo-events of its own, `open` and `close`, emitted by the
socket client when the connection opens or drops, so hooks can refetch after a
reconnect and the call manager can start its grace timer.

### Client to server

| `type` | Fields | Effect |
| --- | --- | --- |
| `ping` | | A `pong` back; the app does not send these today, the transport-level ping/pong keeps the connection alive. |
| `call.join` | `user_id` or `group_id` or `room`, `video` | Start or join a call ([calls.md](calls.md)). |
| `call.leave` | `room` | Leave it. |
| `call.decline` | `room`, `reason?` | Decline a ring. |
| `call.offer`, `call.answer`, `call.ice`, `call.streams` | `room`, `to`, payload | Relayed to `to` if both are in the room. |

An unknown type is logged and ignored.

## The client

`SocketClient` in [`ws.ts`](../mobile/src/lib/ws.ts) is small on purpose:

- `connect(token)` opens `WS_URL` with `['ami', token]` as the subprotocols; `disconnect()` closes for good.
- Every frame is parsed and dispatched by its `type` to the handlers registered
  with `on(type, handler)`, which returns the unsubscribe function. Hooks call
  it in effects and return the unsubscribe as the cleanup.
- `send(msg)` returns false when not connected; callers treat that as an error
  (the call manager ends the call with "Not connected").
- On a drop that the app did not ask for, it reconnects with exponential
  backoff: 1 s, 2 s, 4 s... capped at 30 s, reset on success.

`startRealtime()` ([`realtime.ts`](../mobile/src/lib/realtime.ts)) registers the
app-wide listeners once, before the first connect: the unread store, the
presence store and the call manager. Screens register their own while mounted,
through `useLiveLoad` (reload on focus and on named events), `useChat`,
`usePosts`, `useComments` and the Friends tab.

Reconnect strategy: the hub keeps no backlog, so events during a gap are lost.
Screens that care refetch on `open` (`useChat` merges the newest page;
`useLiveLoad` lists reload on their next focus or event; the call manager
re-joins its room to keep its seat and learn who came and went).
