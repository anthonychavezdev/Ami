# Routes

Every route the server serves, with a one-line summary and a link to the page
that explains it in full. The route table itself is `routes()` in
[`server/main.go`](../server/main.go).

## Conventions

- **Base URL**: the server's origin, `http://localhost:8080` in development. The
  app works it out in [`mobile/src/lib/config.ts`](../mobile/src/lib/config.ts).
- **Authentication**: every route marked *session* needs `Authorization: Bearer <token>`,
  where the token came from `/api/register` or `/api/login`. The WebSocket passes
  it as a handshake subprotocol instead (`Sec-WebSocket-Protocol: ami, <token>`);
  a token in a query string is ignored. A missing or expired token is
  `401 {"error":"unauthorized"}`.
  While an account has a temporary password, every session route except
  `GET /api/me`, `PUT /api/me/password` and `POST /api/logout` answers
  `403 {"error":"set a new password first"}` ([passwords.md](passwords.md#the-forced-change)).
- **Ids** in paths, bodies and events are UUIDs, 36-character strings like
  `019a0c2e-7b3a-7c21-9f4e-6a0d3b1c2e55`. A malformed one in a path is a 400.
- **Bodies** are JSON both ways, except the upload and `/media/`, which carry
  the file itself. Request bodies are capped at 1 MB; uploads at 20 GB.
- **Errors** are `{"error": "<message>"}` with the status in the table below.
  Internal errors are always `500 {"error":"internal error"}`.
- **Pagination**: `?before=<id>&limit=<n>` on lists marked *paged*: items older
  than the one whose id is `before` (ids sort by creation time), newest first, at most `limit` (default 30, max 100; chats
  default 50). Comments use `?after=<id>`, oldest first, up to 500.
- **Rate limits**: 10 register/login attempts per minute per IP; 600 requests per
  minute per IP on `/api/` (`-api-rate`); 30 uploads per minute per user. Over
  the limit is `429` with `Retry-After: 60`.
- **CORS**: browsers need their origin allowed (`-allowed-origins`, or anything
  with `-dev`); the native app sends no `Origin` and needs nothing.
  See [security.md](security.md#browser-origins).

| Status | Meaning here |
| --- | --- |
| 200 | Done; the body is the result. |
| 201 | Created (posts, comments, messages, groups, uploads). |
| 204 | Done, no body (logout, deletes, push tokens). |
| 400 | Invalid input; the message says what. |
| 401 | No valid session, or wrong credentials on login. |
| 403 | Not allowed: not a member, not the author, origin refused, temporary password. |
| 404 | No such thing, or not yours to see (a message outside the chat you named). |
| 409 | Conflict: username taken, friend request already sent, already friends. |
| 413 | Upload over 20 GB. |
| 415 | Upload is not a supported picture or video. |
| 429 | Rate limited. |
| 507 | No disk space left for the upload. |

## Health and the web build

| Route | Auth | Summary | Details |
| --- | --- | --- | --- |
| `GET /api/health` | none | `{"status":"ok","registration":"open"}`, or `"invite"` when registering needs the server's invite code; for load balancers, scripts and the register screen. | [server.md](server.md) |
| `GET /*` | none | Only with `-web-dir`: serves the exported browser build, any unknown path gets `index.html` for the client-side router; `/api/*` paths that match nothing are 404. | [deployment.md](deployment.md#serving-the-web-app) |

## Accounts and sessions

| Route | Auth | Request | Response | Summary | Details |
| --- | --- | --- | --- | --- | --- |
| `POST /api/register` | none, throttled | `{username, password, display_name?, invite?}` | `{token, user: Me}` | Creates an account and signs it in. 403 without the invite code when the server wants one, 400 on bad input, 409 when the username is taken. | [login.md](login.md#post-apiregister) |
| `POST /api/login` | none, throttled | `{username, password}` | `{token, user: Me}` | Signs in; 401 for a wrong username or password. | [login.md](login.md#post-apilogin) |
| `POST /api/logout` | session | | 204 | Ends this session (other devices stay signed in). | [login.md](login.md#post-apilogout) |
| `GET /api/me` | session | | `Me` | The signed-in account with its private settings. | [login.md](login.md#get-apime) |
| `PUT /api/me/password` | session | `{current, new}` | `Me` | Changes the password and signs out every other device; the only write allowed with a temporary password. | [passwords.md](passwords.md#put-apimepassword) |

## Profile

| Route | Auth | Request | Response | Summary | Details |
| --- | --- | --- | --- | --- | --- |
| `PUT /api/me` | session | `{username?, display_name?, bio?, pronouns?, interests?, links?, avatar_url?, avatar_crop?, cover_url?, cover_crop?}` | `Me` | Changes the fields sent; the rest stay. | [profile.md](profile.md#put-apime) |
| `PUT /api/me/presence` | session | `{invisible}` | `{invisible}` | Appear offline to everyone, or visible again. | [profile.md](profile.md#put-apimepresence) |
| `GET /api/stats` | session | | `Stats` | The server's disk, load and memory, and your share of the uploads. | [stats.md](stats.md) |
| `GET /api/ice` | session | | `{ice_servers, ttl}` | The STUN and TURN servers for a call, with TURN credentials for you that last a day. | [calls.md](calls.md#get-apiice) |

## Users

| Route | Auth | Request | Response | Summary | Details |
| --- | --- | --- | --- | --- | --- |
| `GET /api/users?q=` | session | | `UserProfile[]` | Up to 25 people whose username, name or interests contain `q`, with your friendship status to each and their interests. | [users.md](users.md#get-apiusersq) |
| `GET /api/users/{id}` | session | | `UserProfile` | Someone's profile: bio, friendship status, friend and post counts. | [users.md](users.md#get-apiusersid) |
| `GET /api/users/{id}/posts` | session, paged | | `Post[]` | Their profile posts (not group posts); public to any signed-in user. | [users.md](users.md#get-apiusersidposts) |

## Posts and reactions

| Route | Auth | Request | Response | Summary | Details |
| --- | --- | --- | --- | --- | --- |
| `GET /api/feed` | session, paged | | `Post[]` | Your posts, your friends' profile posts and the posts of your groups. | [posts.md](posts.md#get-apifeed) |
| `POST /api/posts` | session | `{body, group_id?, attachments?}` | 201 `Post` | A new post; in a group only for members. | [posts.md](posts.md#post-apiposts) |
| `GET /api/posts/{id}` | session | | `Post` | One post as you see it; a private group's post is 404 to a non-member. | [posts.md](posts.md#get-apipostsid) |
| `PUT /api/posts/{id}` | session | `{body, attachments}` | `Post` | The author edits text and attachments. | [posts.md](posts.md#put-apipostsid) |
| `DELETE /api/posts/{id}` | session | | 204 | The author, or the admin of its group, deletes it. | [posts.md](posts.md#delete-apipostsid) |
| `GET /api/posts/{id}/reactions` | session | | `[{emoji, users}]` | Who reacted with what, the most used emoji first. | [posts.md](posts.md#reactions) |
| `PUT /api/posts/{id}/reaction` | session | `{emoji}` | `Post` | Sets your one reaction to the post. | [posts.md](posts.md#reactions) |
| `DELETE /api/posts/{id}/reaction` | session | | `Post` | Removes it. | [posts.md](posts.md#reactions) |

## Comments

| Route | Auth | Request | Response | Summary | Details |
| --- | --- | --- | --- | --- | --- |
| `GET /api/posts/{id}/comments?after=` | session | | `Comment[]` | A post's comments, oldest first, deleted ones as placeholders. | [comments.md](comments.md#get-apipostsidcomments) |
| `POST /api/posts/{id}/comments` | session | `{body, parent_id?}` | 201 `Comment` | A comment, or a reply under another comment of the same post. | [comments.md](comments.md#post-apipostsidcomments) |
| `PUT /api/comments/{id}` | session | `{body}` | `Comment` | The author edits it. | [comments.md](comments.md#put-apicommentsid) |
| `DELETE /api/comments/{id}` | session | | 204 | The author deletes it; one with replies becomes `[deleted]`. | [comments.md](comments.md#delete-apicommentsid) |
| `GET /api/comments/{id}/reactions` | session | | `[{emoji, users}]` | Who reacted to the comment with what. | [comments.md](comments.md#reactions) |
| `PUT /api/comments/{id}/reaction` | session | `{emoji}` | `Comment` | Sets your one reaction to the comment. | [comments.md](comments.md#reactions) |
| `DELETE /api/comments/{id}/reaction` | session | | `Comment` | Removes it. | [comments.md](comments.md#reactions) |

## Media

| Route | Auth | Request | Response | Summary | Details |
| --- | --- | --- | --- | --- | --- |
| `POST /api/media` | session | the file, with its `Content-Type` | 201 `Attachment` | Uploads one picture or video (JPEG, PNG, GIF, WebP, MP4, MOV, WebM, 20 GB max). 413 when it would pass the account's storage cap. | [media.md](media.md#post-apimedia) |
| `GET /api/media/{name}/reactions` | session | | `[{emoji, users}]` | Who reacted to the upload itself with what. | [media.md](media.md#reactions-on-uploads) |
| `PUT /api/media/{name}/reaction` | session | `{emoji}` | `[{emoji, users}]` | Sets your one reaction to the upload. | [media.md](media.md#reactions-on-uploads) |
| `DELETE /api/media/{name}/reaction` | session | | `[{emoji, users}]` | Removes it. | [media.md](media.md#reactions-on-uploads) |
| `GET /media/{name}` | none | | the file | Serves an upload; names are random, so no session is needed. | [media.md](media.md#get-medianame) |

## Friends

| Route | Auth | Request | Response | Summary | Details |
| --- | --- | --- | --- | --- | --- |
| `GET /api/friends` | session | | `User[]` | Your friends, by display name. | [friends.md](friends.md#get-apifriends) |
| `GET /api/friends/requests` | session | | `{incoming: User[], outgoing: User[]}` | Requests waiting for you, and the ones you sent. | [friends.md](friends.md#get-apifriendsrequests) |
| `POST /api/friends/{id}/request` | session | | `{friend_status}` | Asks to be friends; answers an incoming request by accepting it. | [friends.md](friends.md#post-apifriendsidrequest) |
| `POST /api/friends/{id}/accept` | session | | `{friend_status}` | Accepts their request. | [friends.md](friends.md#post-apifriendsidaccept) |
| `DELETE /api/friends/{id}` | session | | `{friend_status: "none"}` | Cancels your request, declines theirs, or unfriends. | [friends.md](friends.md#delete-apifriendsid) |

## Groups

| Route | Auth | Request | Response | Summary | Details |
| --- | --- | --- | --- | --- | --- |
| `GET /api/groups?mine=1` | session | | `Group[]` | The groups you belong to. | [groups.md](groups.md#get-apigroups) |
| `GET /api/groups?q=` | session | | `Group[]` | Public groups to discover, filtered by name (50 max). | [groups.md](groups.md#get-apigroups) |
| `POST /api/groups` | session | `{name, description?, private?, member_ids?}` | 201 `GroupDetail` | A new group with you as owner and admin, optionally with friends added. | [groups.md](groups.md#post-apigroups) |
| `GET /api/groups/{id}` | session | | `GroupDetail` | The group, its members and who is in its call; a private group is 404 to a non-member. | [groups.md](groups.md#get-apigroupsid) |
| `DELETE /api/groups/{id}` | session | | 204 | The owner deletes it, with its posts, chat and call. | [groups.md](groups.md#delete-apigroupsid) |
| `POST /api/groups/{id}/join` | session | | `GroupDetail` | Join a public group. | [groups.md](groups.md#post-apigroupsidjoin) |
| `POST /api/groups/{id}/leave` | session | | `GroupDetail` | Leave (the owner cannot). | [groups.md](groups.md#post-apigroupsidleave) |
| `GET /api/groups/{id}/posts` | session, paged | | `Post[]` | The group's posts; a private group's are members only. | [groups.md](groups.md#get-apigroupsidposts) |
| `POST /api/groups/{id}/members` | session | `{user_ids}` | `GroupDetail` | The admin adds friends. | [groups.md](groups.md#post-apigroupsidmembers) |
| `DELETE /api/groups/{id}/members/{uid}` | session | | `GroupDetail` | The admin removes a member. | [groups.md](groups.md#delete-apigroupsidmembersuid) |

## Chats

Direct chats live under `/api/messages/{userId}`, group chats under
`/api/groups/{id}/messages`; the handlers behind them are the same.

| Route | Auth | Request | Response | Summary | Details |
| --- | --- | --- | --- | --- | --- |
| `GET /api/conversations` | session | | `Conversation[]` | Every direct chat with messages and every group you are in, most recent first, with unread counts. | [messages.md](messages.md#get-apiconversations) |
| `GET /api/messages/{userId}` | session, paged | | `Message[]` | The newest messages with a friend, oldest first within the page. | [messages.md](messages.md#listing-messages) |
| `POST /api/messages/{userId}` | session | `{body, attachments?}` | 201 `Message` | Sends a message to a friend. | [messages.md](messages.md#sending) |
| `POST /api/messages/{userId}/read` | session | | `{unread}` | Marks the chat read; returns your new unread total. | [messages.md](messages.md#reading) |
| `PUT /api/messages/{userId}/{mid}` | session | `{body}` | `Message` | Edits your own message. | [messages.md](messages.md#editing) |
| `DELETE /api/messages/{userId}/{mid}` | session | | 204 | Deletes your own message for everyone. | [messages.md](messages.md#deleting) |
| `GET /api/messages/{userId}/{mid}/reactions` | session (a party) | | `[{emoji, users}]` | Who reacted to the message with what. | [messages.md](messages.md#reactions) |
| `PUT /api/messages/{userId}/{mid}/reaction` | session (a party) | `{emoji}` | `Message` | Sets your one reaction to the message. | [messages.md](messages.md#reactions) |
| `DELETE /api/messages/{userId}/{mid}/reaction` | session (a party) | | `Message` | Removes it. | [messages.md](messages.md#reactions) |
| `GET /api/groups/{id}/messages` | session (member), paged | | `Message[]` | The group chat, each message with its sender. | [messages.md](messages.md#listing-messages) |
| `POST /api/groups/{id}/messages` | session (member) | `{body, attachments?}` | 201 `Message` | Writes in the group chat. | [messages.md](messages.md#sending) |
| `POST /api/groups/{id}/messages/read` | session (member) | | `{unread}` | Marks the group chat read. | [messages.md](messages.md#reading) |
| `PUT /api/groups/{id}/messages/{mid}` | session (member) | `{body}` | `Message` | Edits your own message. | [messages.md](messages.md#editing) |
| `DELETE /api/groups/{id}/messages/{mid}` | session (member) | | 204 | Deletes your own message, or any message as the admin. | [messages.md](messages.md#deleting) |
| `GET /api/groups/{id}/messages/{mid}/reactions` | session (member) | | `[{emoji, users}]` | Who reacted to the message with what. | [messages.md](messages.md#reactions) |
| `PUT /api/groups/{id}/messages/{mid}/reaction` | session (member) | `{emoji}` | `Message` | Sets your one reaction to the message. | [messages.md](messages.md#reactions) |
| `DELETE /api/groups/{id}/messages/{mid}/reaction` | session (member) | | `Message` | Removes it. | [messages.md](messages.md#reactions) |

## Push tokens

| Route | Auth | Request | Response | Summary | Details |
| --- | --- | --- | --- | --- | --- |
| `PUT /api/push-token` | session | `{token}` | 204 | Registers this device's Expo push token. | [notifications.md](notifications.md#put-apipush-token) |
| `DELETE /api/push-token` | session | `{token}` | 204 | Forgets it (logout). | [notifications.md](notifications.md#delete-apipush-token) |

## WebSocket

| Route | Auth | Summary | Details |
| --- | --- | --- | --- |
| `GET /ws` | session (subprotocol) | Upgrades to the realtime connection: a `hello` event, then every event the account is an audience of, and the call signaling channel. | [websocket.md](websocket.md), [calls.md](calls.md) |

Client messages over the socket are `{"type":"ping"}` and the `call.*`
messages; server events are listed in [websocket.md](websocket.md#events).
