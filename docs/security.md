# Security

What protects what, where it lives, and what is deliberately not there. The
defaults are production defaults; development extras are opt-in with `-dev`.

## Accounts

- Passwords are hashed with PBKDF2-HMAC-SHA256, 600,000 rounds, a random
  16-byte salt per password, and compared in constant time. The round count is
  stored per user and raised at login when the constant grows
  ([login.md](login.md)). Register and login are limited to 10 attempts per
  minute per IP, login also to 10 per minute per account from any number of
  addresses, and a wrong username and a wrong password are the same 401.
- Sessions are 256-bit random tokens stored only as SHA-256 hashes, expire after
  30 days, and are purged hourly. A password change signs out every other
  device; an operator reset signs out all of them and restricts the account to
  setting a new password ([passwords.md](passwords.md)).
- There is no email, so no verification and no self-service recovery; the
  operator resets passwords in person.
- Registration is open until the operator sets a standing invite code
  (`-invite`, `AMI_INVITE`) or makes the first one-time code with
  `ami-server -new-invite`, which works against the running server's database.
  From then on `POST /api/register` needs the standing code or an unspent
  one-time code, checked before the username, so an uninvited caller learns
  nothing about usernames. One-time codes are 80 random bits, kept hashed,
  spent by the account they create and remembered afterwards, so registration
  never opens again by itself. The register rate limit is what stands against
  guessing; a standing code is shared, so make it long. The app asks
  `GET /api/health` whether to show the field.

## Authorization

Every rule is enforced by the server, in the handler that performs the action:

| Resource | Who may |
| --- | --- |
| A profile, profile posts, a post by id, a public group's detail and posts | any signed-in user |
| A private group's detail, members and posts, with their comments and reactions | members (anyone else gets a 404) |
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
| The server's disk, load and memory, and your own share of the uploads (`/api/stats`) | any signed-in user |

The listings scope what people discover (the feed, group discovery), but
fetching a profile post, a public group or a profile by id is open to any
signed-in user; see [Not done](#not-done). Private groups are the exception:
`viewingGroup` guards the group routes and `viewingPost`/`viewingComment`
guard every post and comment route (`mayViewPost`), so a non-member gets a
404 for the group, its posts, and the comments and reactions on them.

## Browser origins

CORS and the WebSocket upgrade share one policy (`originAllowed`): requests
without an `Origin` (the native app, curl), the page's own origin (the web build
served by `-web-dir`), and the `-allowed-origins` list. Other origins get no
CORS headers and a 403 on preflight and on the socket upgrade. `-dev` allows
any origin unless a list is given.

## Headers

Every response gets `X-Content-Type-Options: nosniff`, `Cache-Control: no-store`,
`Referrer-Policy: no-referrer`, `Cross-Origin-Resource-Policy: same-site` (a
page on another site cannot embed an upload, random name or not),
`X-Frame-Options: DENY`,
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
| Upload | 20 GB, streamed to disk; a transfer that stalls for 2 min is dropped | `uploadMedia` |
| Request headers | 64 KB | `http.Server` |
| WebSocket message | 64 KB | `wsUpgrade` |
| WebSocket messages | 300 per 10 s per connection | `handleWS` |
| Auth attempts | 10 per minute per IP, and login 10 per minute per account | `throttle`, `login` |
| Sockets | 16 open per account; the handshake counts against the API budget | `handleWS`, `apiLimit` |
| API requests | 600 per minute per IP (`-api-rate`) | `apiLimit` |
| Uploads | 30 per minute per user | `uploadMedia` |
| Storage | `-user-storage-gb` per account in total, counting every upload whether attached or not (off by default) | `uploadMedia` |
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
auth and push-token routes, the `Authorization` header and any `token`
query parameter redacted, bodies that hold credentials but cannot be parsed
hidden entirely, and uploads shown by size only ([server.md](server.md#logging)).

## Transport

TLS 1.2 is the minimum when the server terminates TLS. Behind a proxy,
`-trust-proxy` must only be set when the proxy is the sole route to the port,
since it makes the server believe `X-Forwarded-For` and `X-Forwarded-Proto`.
The client address is then the *last* `X-Forwarded-For` hop, the one the
proxy appended, so a client sending the header itself gains nothing; with a
chain of proxies, have the inner one pass a single hop. Session tokens travel
only in the `Authorization` header and, on the WebSocket handshake where
browsers cannot set headers, in the `Sec-WebSocket-Protocol` subprotocols
(`ami, <token>`); never in a URL, which proxies and browsers log. The server
should run on HTTPS in production.

## Calls

The server authorises who may enter a room (friends, members) and only relays
signaling between seated participants. Media is encrypted end to end by WebRTC
(DTLS-SRTP) and never passes through the server. When two networks cannot
reach each other, the media is relayed by the operator's coturn, which the
server vouches for with credentials that expire after a day
([calls.md](calls.md#get-apiice)); without one, such calls fail rather than
fall back to the server. coturn itself should relay only to public addresses
and with quotas ([deployment.md](deployment.md#turn)), since any account can
ask it to relay.

## Not done

Things a reader should know are absent, so they are not assumed:

- No email, verification, two-factor authentication, account lockout beyond the
  per-IP rate limit, or audit log.
- Profile posts, public groups and profiles are fetchable by id by any
  signed-in user who knows the id; privacy is at the level of listings, except
  for private groups, whose detail and posts are members only. Ids are random
  UUIDs, so they cannot be enumerated, but a shared link reveals one.
- Uploads that are never attached are kept forever. `-user-storage-gb` caps
  what one account may upload in total; without it the only bound is the
  rate limit and the disk.
- In the browser the session token lives in `localStorage` (phones keep it in
  the Keychain or Keystore), so the page's CSP, which allows only its own
  scripts, is what stands between a script injection and the token.
- Push notifications carry the sender's name and the message text through
  Expo's push service and Apple's and Google's, so a message is not private
  from those services while a recipient's app is closed
  ([notifications.md](notifications.md)).
- Rate limits and the hub are per process; a reverse proxy in front should add
  its own limits for a public deployment.
- Markdown is rendered client-side by a library configured to ignore HTML; the
  server stores the text as is.
