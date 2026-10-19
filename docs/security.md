# Security

What protects what, where it lives, and what is deliberately not there. The
defaults are production defaults; development extras are opt-in with `-dev`.

## Accounts

- Passwords are hashed with PBKDF2-HMAC-SHA256, 600,000 rounds, a random
  16-byte salt per password, and compared in constant time. The round count is
  stored per user and raised at login when the constant grows
  ([login.md](login.md)). Register and login are limited to 10 attempts per
  minute per IP, and a wrong username and a wrong password are the same 401.
- Sessions are 256-bit random tokens stored only as SHA-256 hashes, expire after
  30 days, and are purged hourly. A password change signs out every other
  device; an operator reset signs out all of them and restricts the account to
  setting a new password ([passwords.md](passwords.md)).
- There is no email, so no verification and no self-service recovery; the
  operator resets passwords in person.

## Authorization

Every rule is enforced by the server, in the handler that performs the action:

| Resource | Who may |
| --- | --- |
| A profile, profile posts, a post by id, a group's detail and posts | any signed-in user |
| Edit a post or comment | its author |
| Delete a post | its author, or the admin of its group |
| Delete a comment | its author |
| Comment, react | any signed-in user who can fetch the post |
| Message or call someone | friends only |
| Read or write a group chat, post in a group, join its call | members |
| Delete a group message | its sender, or the group's admin |
| Add members, remove members | the admin (never the owner; only friends can be added) |
| Join | public groups only |
| Delete a group | the owner |
| Edit your profile, change your password, register push tokens | you |

The listings scope what people discover (the feed, group discovery), but
fetching a post, a group or a profile by id is open to any signed-in user; see
[Not done](#not-done).

## Browser origins

CORS and the WebSocket upgrade share one policy (`originAllowed`): requests
without an `Origin` (the native app, curl), the page's own origin (the web build
served by `-web-dir`), and the `-allowed-origins` list. Other origins get no
CORS headers and a 403 on preflight and on the socket upgrade. `-dev` allows
any origin unless a list is given.

## Headers

Every response gets `X-Content-Type-Options: nosniff`, `Cache-Control: no-store`,
`Referrer-Policy: no-referrer`, `X-Frame-Options: DENY`,
`Content-Security-Policy: default-src 'none'; frame-ancestors 'none'`, a
`Permissions-Policy` denying camera, microphone, display capture and geolocation,
`Cross-Origin-Opener-Policy: same-origin`, and HSTS when served over TLS or
behind a trusted proxy that says so. Two handlers relax this on purpose:

- the web build (`web.go`): a CSP allowing the page's own scripts, inline styles
  (react-native-web), `blob:` and `data:` media and fonts, and WebSocket
  connections to the same host, with a `Permissions-Policy` that allows the
  camera, microphone and display capture for the page itself;
- media (`media.go`): a sandboxing CSP (`default-src 'none'; sandbox`) and an
  immutable cache, so an upload opened as a document can do nothing.

## Uploads

Files are sniffed, never trusted by name or declared type; only JPEG, PNG, GIF,
WebP, MP4, MOV and WebM pass, and SVG is not on the list. Each file gets a
random, unguessable name with the extension of the sniffed type, created with
`O_EXCL`. Served files are limited to the media directory (no separators, no
dot-prefixed names in the path) and sent with the sandboxing CSP. Attachments on
posts and messages must reference uploads that exist, so no response ever
carries a URL that was not produced by the server, except links, which must be
`http(s)` and are shown as plain cards, never fetched or embedded
([media.md](media.md)).

## Limits

| What | Limit | Where |
| --- | --- | --- |
| JSON request body | 1 MB | `readJSON` |
| Upload | 64 MB | `uploadMedia` |
| Request headers | 64 KB | `http.Server` |
| WebSocket message | 64 KB | `wsUpgrade` |
| WebSocket messages | 300 per 10 s per connection | `handleWS` |
| Auth attempts | 10 per minute per IP | `throttle` |
| API requests | 600 per minute per IP (`-api-rate`) | `apiLimit` |
| Uploads | 30 per minute per user | `uploadMedia` |
| Post text | 5,000 bytes | `cleanPostInput` |
| Message text | 4,000 bytes | `cleanMessage` |
| Comment text | 1 to 2,000 bytes | `validCommentBody` |
| Attachments | 10 per post or message, URLs of 2,048 bytes | `cleanAttachments` |
| Group name, description | 60 and 500 bytes | `cleanGroupInput` |
| Members added at once | 100 | `friendsToAdd` |
| Username, password | 3 to 24 characters; 8 to 128 bytes | `validCredentials` |
| Display name, bio, pronouns | 50, 500, 30 characters | `profilePatch` |
| Interests | 20 tags of 30 characters | `cleanInterests` |
| Profile links | 10, labels of 30 characters, URLs of 2,048 bytes | `cleanLinks` |
| Emoji | 32 bytes, 8 runes | `validEmoji` |
| List pages | 100 items (comments 500) | `pagination`, `listComments` |
| Call | 8 participants | `maxCallParticipants` |
| Timeouts | header 10 s, read 30 s, write 60 s, idle 2 min, shutdown grace 10 s, socket pong 90 s | `serve`, `ws.go` |

## Errors and logs

Internal errors are logged and answered with a generic 500; clients never see
SQL or stack details. The normal log holds method, path, status and duration
only. `-verbose` logs bodies for debugging, with the credential fields of the
auth and push-token routes, the `Authorization` header and the socket's
`?token=` redacted, bodies that hold credentials but cannot be parsed hidden
entirely, and uploads shown by size only ([server.md](server.md#logging)).

## Transport

TLS 1.2 is the minimum when the server terminates TLS. Behind a proxy,
`-trust-proxy` must only be set when the proxy is the sole route to the port,
since it makes the server believe `X-Forwarded-For` and `X-Forwarded-Proto`.
Session tokens travel in the `Authorization` header, or in the `?token=` query
of the WebSocket handshake (browsers cannot set headers there), which is why
the verbose log redacts that parameter and why the server should run on HTTPS
in production.

## Calls

The server authorises who may enter a room (friends, members) and only relays
signaling between seated participants. Media is encrypted end to end by WebRTC
(DTLS-SRTP) and never passes through the server. The only STUN server
configured is Google's public one; with no TURN server, some network pairs
cannot connect at all, but nothing falls back to relaying through the server.

## Not done

Things a reader should know are absent, so they are not assumed:

- No email, verification, two-factor authentication, account lockout beyond the
  per-IP rate limit, or audit log.
- Reserved usernames (`deleted`, `admin`, `ami`, `me`, `system`) are refused when
  renaming, but not at registration.
- Posts, groups and profiles are fetchable by id by any signed-in user who knows
  the id; privacy is at the level of listings and of group chats and calls, not
  of single objects. Ids are random UUIDs, so they cannot be enumerated, but a
  shared link reveals one.
- Uploads that are never attached are kept forever; there is no quota per user
  beyond the rate limit.
- Rate limits and the hub are per process; a reverse proxy in front should add
  its own limits for a public deployment.
- Markdown is rendered client-side by a library configured to ignore HTML; the
  server stores the text as is.
