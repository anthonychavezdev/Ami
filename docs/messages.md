# Messages

Direct chats between friends and group chats, how they are read and counted,
and how the app keeps them live. Code: [`server/messages.go`](../server/messages.go);
in the app [`useChat.ts`](../mobile/src/lib/useChat.ts),
[`messages.ts`](../mobile/src/lib/messages.ts),
[`ChatView`](../mobile/src/components/ChatView.tsx),
[`MessageBubble`](../mobile/src/components/MessageBubble.tsx), the Messages tab
([`(tabs)/messages.tsx`](../mobile/src/app/(tabs)/messages.tsx)),
[`ConversationRow`](../mobile/src/components/ConversationRow.tsx) and the unread
store [`unread.ts`](../mobile/src/lib/unread.ts).

## The shapes

```json
Message:      { "id": "019a0c3a-…", "sender_id": "019a0b10-…", "recipient_id": "019a0b11-…", "group_id": null, "sender": {"...User"},
                "body": "...", "attachments": [], "created_at": "...", "edited_at": null }
Conversation: { "user": {"...User"}, "group": Group, "last_message": Message, "unread": 2 }
```

A message is **direct** when `recipient_id` is set and a **group** message when
`group_id` is set, never both. `sender` is included on group messages only
(several people write there); a direct chat already knows who the other person
is. A `Conversation` has either `user` or `group`, and `last_message` is `null`
for a group nobody has written in.

## Addressing a chat

The same handlers serve both kinds. Two adapters turn a route into a `chat`
value (`{peer}` or `{group}`):

- `directChat`: `/api/messages/{id}` names the other person.
- `groupChat`: `/api/groups/{id}/messages` names the group and refuses
  non-members with `403 only members can use the group chat`.

`chat.filter()` is the SQL that selects a chat's messages, `chat.contains(m)`
checks that a message named by id belongs to it (a group message is 404 through
the direct route and vice versa), and `chat.readEvent(unread)` builds the
`messages.read` event for it.

## Listing messages

`GET` on the chat: the newest messages, 50 by default (`?limit=` up to 100),
`?before=<id>` for older ones. The page is returned **oldest first**, as a chat
reads; the app reverses it for its inverted list.

## Sending

`POST` with `{"body": "...", "attachments": [...]}`. Text or at least one
attachment is required, text is capped at 4,000 bytes, attachments follow the
usual rules ([media.md](media.md#attachments-on-posts-and-messages)). A direct
message needs friendship (`403 you can only message friends`).

After the insert, `deliverMessage` pushes the `message` event:

- to the sender's own connections (their other devices and the sending one)
  as a plain copy, without an unread count;
- to every other member of the chat with their own new unread total
  (`unread` on the event), and, for anyone **with no live connection**, a push
  notification through Expo ([notifications.md](notifications.md)).

The response is `201` with the message, which the app applies at once.

## Reading

`POST .../read` moves your read marker for the chat to its newest message
(`moveReadMarker`, an upsert into `conversation_reads` or `group_reads`),
recomputes your total unread across all chats, sends your own devices
`messages.read {peer_id | group_id, unread}` so their badges drop too, and
answers `{"unread": N}`.

## Editing

`PUT .../{mid}` with `{"body": "..."}` on your own message (`403 only the sender
can edit a message`); attachments stay as they were. `edited_at` is set and
everyone in the chat gets `message.updated` with the message.

## Deleting

`DELETE .../{mid}`: your own message, or, in a group, any message as the admin
(`mayDeleteMessage`). The row is deleted and each member of the chat gets `message.deleted {id, sender_id,
recipient_id, group_id, unread}` with their own recomputed total. 204.

## Unread counts

A message is unread for a user when its id is above that user's read marker for
the chat, and, in a group, when someone else wrote it. `unreadMessages(user)`
sums both kinds in one query; `groupUnread` counts one group. The markers:

| Table | Key | Set by |
| --- | --- | --- |
| `conversation_reads` | `(user_id, peer_id)` | reading a direct chat; migration 1 marks chats older than unread tracking as read |
| `group_reads` | `(user_id, group_id)` | reading the group chat; joining or being added (read up to now); removed on leaving |

Ids are time-ordered UUIDs that are never reused, so a marker left at a
deleted message stays right: anything said later has a greater id. An empty
marker means nothing was read.

Leaving a group, being removed, or the group being deleted sends the affected
users a `messages.read` for that group with their new total (`refreshBadge`).

## GET /api/conversations

The Messages tab's list: for direct chats, the newest message with each person
(one row per peer, found with `MAX(id) ... GROUP BY peer`) and the unread count;
for groups, every group you belong to with its newest message if any. Sorted
most recent message first; silent groups last, by name.

## In the app

### `useChat(kind, id)`

Everything a chat screen needs:

- Loads the other person (`api.user`) or the group detail (members and who is
  in its call) together with the newest page of messages, reversed for the
  inverted list, and marks the chat read on open and whenever the app returns
  to the foreground while it is open.
- Tells the unread store which chat is open (`unread.setActiveChat`), so a
  message arriving for it is marked read instead of notified.
- Applies `message`, `message.updated` and `message.deleted` events that belong
  to this chat (`belongsTo`), and after a socket reconnect (`open`) fetches the
  newest page again and folds it in with `mergeLatest`, since anything said
  meanwhile was missed.
- For a group, follows `call.*` events for its room to show who is in the call.
- `send`, `edit` and `remove` apply the server's reply at once; the socket echo
  of the same change is a no-op. `loadOlder` pages back from the oldest message.

### `ChatView`

One component for both kinds, as a screen and as the pane of the Messages tab.
The header (native, or a `PaneHeader` in the pane) shows the name with voice and
video call buttons, plus a group details button for groups. A `CallBanner`
appears above a group chat when a call is in progress (*Join*) or you are in it
(*Return*). The list is inverted (newest at the bottom), `useInvertedWheel` fixes
mouse-wheel direction on the web, and reaching the top loads older messages.
`MessageBubble` puts your messages on the right, names the sender in group
chats, shows attachments and the *edited* mark; hold a bubble (devices) or hover
for ⋯ / right-click (web) to edit or delete. Editing swaps the compact
`Composer` into edit mode with an `EditBar` above it; the composer otherwise
sends text and picked pictures or videos.

### The Messages tab and the badge

The tab lists conversations with `ConversationRow`s (friend or group avatar,
the last message with who wrote it, the unread badge) and reloads on focus and
after `message`, `message.updated`, `message.deleted`, `messages.read` and
`group.added` events. The + button starts a new group chat. On a wide window the
open chat sits beside the list, named by `?chat=u:<id>` or `?chat=g:<id>`
(`chatKey` / `parseChatKey`), and `/chat/...` routes redirect there.

The tab bar badge and the OS badge come from the unread store
([`unread.ts`](../mobile/src/lib/unread.ts)): the server's totals arrive in
`hello`, `messages.read`, `message.deleted` and on each `message` for a
recipient; the store decides whether a new message is yours (ignored), for the
chat you are looking at (marked read) or news (count updated and a notification
shown, see [notifications.md](notifications.md)).
