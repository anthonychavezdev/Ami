# Ami

Ami (short for *amigo*) is a small social network: posts, comments, emoji reactions, friend requests,
groups, direct messages, and peer-to-peer voice/video calls with screen sharing.

- `server/` — Go backend over MariaDB (`go-sql-driver/mysql`, pure Go, no
  cgo). HTTP routing, auth (PBKDF2 + session tokens) and WebRTC signaling are
  stdlib; the WebSocket server uses gorilla/websocket.
- `mobile/` — Expo / React Native app (TypeScript, Expo Router, `react-native-webrtc`); runs on Android, iOS and in the browser.
- `devenv.nix` — toolchains, env vars and the `server`, `web`, `android`, `test`, `build`
  scripts. Everything installs inside this repo (`.go/`, `.npm/`,
  `mobile/node_modules/`, `.gradle/`, `.android/`, `.cocoapods/`).
- `docs/` — how everything works, top to bottom: start at [docs/README.md](docs/README.md).

## Run

```sh
devenv shell          # Go, Node, npm, MariaDB, watchman, Android SDK + emulator, JDK 17 (macOS: CocoaPods too)
setup                 # go mod tidy + npm install (first time)
devenv up             # MariaDB (data in .devenv/state/mysql) + the API on :8080 in dev mode
server                # the API alone, against that MariaDB (`devenv up mysql` runs just the database)
android               # build the app, install it on the emulator (created on first run), start Metro
ios                   # same, in the iOS Simulator (macOS with Xcode from the App Store; Linux gets everything but iOS)
mobile                # Metro only — once the app is installed, for day-to-day JS work
web                   # the same app in a browser tab (http://localhost:8081)
test                  # gofmt, go vet, go test (starts MariaDB if none is running); tsc, eslint, prettier --check
```

On Linux the emulator needs `/dev/kvm` access (NixOS: add your user to the
`kvm` group). If `android` reports that no emulator could be started, run
`emulator -avd ami` by hand to see the emulator's own error.

`server -dev` fills an empty database with demo accounts (`ana`, `luis`,
`sofia`, `diego`, `valentina`, `mateo`, `camila`, `javier`, password
`amigo-demo`) plus friendships, groups, posts and chats, and allows any
browser origin, which the web app served by Metro needs. Log in as `ana` to
see every state. `server` alone runs with the production defaults: no demo
data, no CORS.

To watch what a client sends and gets back, add `-verbose` to either
(`server -dev -verbose`): every request and response is logged with headers
and body, and every WebSocket message in and out. Passwords and tokens are
redacted and bodies are cut at 4 KB, so the output is safe to paste into an
issue.

The app needs a development build (not Expo Go) because of `react-native-webrtc`.
`android` produces one and installs it on the emulator, or on a USB-connected
phone if one is plugged in; `ios` does the same in the Simulator. The native
projects (`mobile/android`, `mobile/ios`) are generated: after changing native
dependencies, `app.json` or `mobile/plugins/`, run `npx expo prebuild --clean`
in `mobile/` so the next `android`/`ios` picks the changes up.

## Checks

`test` runs everything that must pass before a commit. The Go tests
(`server/*_test.go`) drive a real server over HTTP and WebSocket on a
temporary database: every route, the realtime events, call signaling and the
upgrade of a first-release database. The app is type-checked, linted
(`eslint-config-expo`) and format-checked (Prettier; `npm run format` fixes
it). `mobile/e2e/` has scripts that drive the Android app on emulators for
what tests cannot cover: screens, WebRTC media and permission dialogs.

### Changing things

- The schema: edit `schema` in `server/db.go` and append a migration to
  `migrations`; a database records how many have run in `PRAGMA user_version`.
- A user column: `userCols` and `User.fields()` in `server/users.go`, the
  schema, and `User` in `mobile/src/lib/types.ts`.
- A realtime event: a struct in `server/events.go` and its twin in
  `mobile/src/lib/events.ts`; `socket.on` and `socket.send` are typed from it.
- Presence: `User.online` is filled in by reflection over every response
  (`server/presence.go`), so handlers never set it.

### Ports and addresses

The defaults are in `devenv.nix` (`env`). Override them in a gitignored `.env`
at the repo root (`NAME=value` per line, loaded when you enter the shell), in
`devenv.local.nix`, or for one command: `AMI_PORT=9000 server`.

| Variable              | Default | What it does                                                                                   |
| --------------------- | ------- | ---------------------------------------------------------------------------------------------- |
| `AMI_PORT`            | `8080`  | Port of the Go API (`server`, `devenv up`) and where the app looks for it.                      |
| `AMI_DB`              | devenv's MariaDB | The server's database, `user:password@tcp(host:3306)/db` or `@unix(/path/mysql.sock)/db`. |
| `AMI_METRO_PORT`      | `8081`  | Metro, which also serves the web app. Native dev builds bake it in: rebuild after changing it.  |
| `AMI_HOST`            | LAN IP  | Address phones use to reach this machine, if Expo picks the wrong interface (VPNs, two networks). |
| `EXPO_PUBLIC_API_URL` | empty   | Full URL of the API when it runs on another machine. Release builds require it (`https://`).   |

Unless `EXPO_PUBLIC_API_URL` is set, the app calls the API on the machine it was
loaded from, at `AMI_PORT`: the web page uses the host in the browser's address
bar, native dev builds use the machine running Metro. So with `server -dev` and `web`
running here, another device just opens `http://<this machine's IP>:8081`.

## Production

`build` writes release artifacts to `dist/`; `build server`, `build web`,
`build android` or `build ios` makes just one:

- `dist/ami-server`: static, stripped binary (set `GOOS`/`GOARCH` to cross-compile).
  Run it with `AMI_DB` pointing at a MariaDB database of its own and either
  `-tls-cert`/`-tls-key`, or behind a TLS-terminating reverse proxy plus
  `-trust-proxy`. Never pass `-dev`.
  Other flags: `-addr`, `-allowed-origins` (CORS is off by default), `-web-dir`.
- `dist/web`: the browser build (minified, hashed bundles). Serve it from the
  same binary with `-web-dir dist/web`: the page then talks to the origin it
  was loaded from, so no CORS setup is needed. If `EXPO_PUBLIC_API_URL` is set
  it is baked in instead (then pass `-allowed-origins https://your.site`).
  Browsers only allow camera and microphone access, so calls, on HTTPS or
  `localhost`.
- `dist/ami-release.apk`: minified release build (Hermes bytecode, R8, resource
  shrinking) signed with your keystore, never the debug key. Required:
  `EXPO_PUBLIC_API_URL=https://…` (baked into the app), `AMI_KEYSTORE`,
  `AMI_KEYSTORE_PASSWORD`; optional `AMI_KEY_ALIAS` (default `ami`).
- `dist/Ami.ipa`: Release archive exported by Xcode with automatic signing.
  Needs Xcode signed in to your Apple Developer account and `AMI_APPLE_TEAM`
  (the 10-character team id). `AMI_IOS_METHOD` picks the export:
  `app-store-connect` (default; upload with Transporter or TestFlight),
  `release-testing` (ad hoc, for registered devices) or `debugging`.

### Forgotten passwords

There is no email, so you are the recovery channel. On the machine with the
database (the server may keep running):

```sh
./ami-server -db /var/lib/ami/ami.db -reset-password luis    # development: server -reset-password luis
```

It prints a temporary password, signs that user out of every device and exits.
Tell them the password in person. Logging in with it only allows setting a new
one: the app opens that screen and the API refuses everything else until it is
done. Anyone can also change their password from the Me tab.

### Notifications

A message to someone who is not looking at that chat shows an OS notification:
in the browser through the Notification API (asked for on login), on Android
and iOS through `expo-notifications` as long as the app is running. For
notifications while the app is closed, the server pushes through Expo's push
service to devices that have no live connection. That needs an EAS project:
`eas init` in `mobile/` (adds `extra.eas.projectId` to `app.json`), then
`eas credentials` to upload FCM (Android) and APNs (iOS) keys. Without a
project id the app simply never registers a push token.

### Security

- Passwords: PBKDF2-HMAC-SHA256, 600,000 rounds, per-user salt; older hashes
  are upgraded on login. Session tokens are random, stored hashed, expire after
  30 days and are purged hourly.
- Browser origins: CORS and the WebSocket upgrade accept the page's own origin
  (`-web-dir`), the `-allowed-origins` list, and requests without an Origin
  (the native app). `-dev` allows any origin unless a list is given.
- Rate limits: 10 login/register attempts per minute per IP, 600 API requests
  per minute per IP (`-api-rate`, 0 disables), 30 uploads per minute per user,
  300 WebSocket messages per 10 s per connection.
- Headers: `nosniff`, `no-store`, `no-referrer`, `X-Frame-Options: DENY`, a
  strict `Content-Security-Policy` (relaxed only for the web build and media),
  `Permissions-Policy`, `Cross-Origin-Opener-Policy`, and HSTS when serving TLS
  or behind a proxy with `-trust-proxy`.
- Limits: 1 MB JSON bodies, 20 GB uploads (streamed to disk, content sniffed,
  random names, no SVG), 64 KB WebSocket messages, 64 KB headers, read/write/idle timeouts,
  TLS 1.2+. Internal errors are logged, never returned to clients.

## Features

| Area | Notes |
|---|---|
| Posts | Markdown text, pictures, videos and links, together or on their own; tap a picture or video to view it fullscreen (arrows, thumbnails, Save to your library). Authors can edit their posts (a pencil marks edited ones). Comments thread like Reddit: reply under a comment, edit or delete your own; a deleted comment with replies stays as [deleted]. In the browser, your posts, comments and messages get a ⋯ button on hover and a right-click menu. Feed = your posts + friends' posts + posts in your groups. Profile posts are public. |
| Reactions | Tap **React** to 👍; press-and-hold opens the emoji menu. One reaction per user per post, comment or chat message, any emoji. Rest the pointer on a reaction to see who gave it; on a phone, hold it. |
| Comments | On the post screen. |
| Server stats | A Server tab with the disk the uploads sit on and your share of them, the processor's load and the memory, so everyone can see what they are taking up. |
| Profile | A picture and a wide cover (GIFs animate), each with its own crop; display name, pronouns, a Markdown bio, interests as tags that Find people searches, links to your other places; a username change that must be valid and free, and a password change (other devices are signed out). |
| Friends | Request → accept. Only friends can message/call each other. Pushed live over WebSocket. Every avatar carries a presence dot: green while the user has the app open, gray otherwise. "Appear offline" on the Me tab hides yours. |
| Groups | Creator is owner + admin; others join as members. Admin can add friends, remove members, delete posts, delete the group. Every group has a chat and a call. Invite-only groups are hidden from Discover and only the admin adds people; that is what the + button on the Messages tab creates. |
| Messages | Direct messages and group chats, with pictures and videos, delivered in real time. Group messages show who wrote them; the group admin can delete any of them. Hold a message to react, or to edit or delete your own. Unread badges on the Messages tab (per chat and in total) and OS notifications (browser, Android, iOS) that name the group. |
| Calls | Voice or video, one-to-one or in a group (up to 8 people). WebRTC with STUN (`stun.l.google.com`), full mesh: everyone sends to everyone, the server only relays signaling. A group call rings every member; the group chat shows *Call in progress · Join* while it is on. Mute, camera on/off, flip camera. |
| Screen share | Toggle during any call, group calls included; sent as a second video stream to every participant and shown on a stage above the people. |
| Theme | Light, dark, or follow the device setting (Me tab). |
| Wide screens | In a browser on a laptop or desktop (and on tablets) the tabs become a sidebar, every screen sits in a centred column of readable width, and every tab shows its list beside the open post, chat, group or profile, the Me tab its settings beside your own profile or the form being filled in; phones keep the bottom tabs and full-screen chats. |

### Screen sharing caveats

- **Android**: works out of the box (the config plugin in `mobile/plugins/`
  enables react-native-webrtc's media-projection foreground service).
- **iOS**: requires adding a *Broadcast Upload Extension* target in Xcode after
  `npx expo prebuild` — see react-native-webrtc's iOS screen-sharing guide.
- **Audio**: `getDisplayMedia({ audio: true })` is requested, so where the
  platform supports it (web/desktop browsers) the shared window's audio is sent.
  react-native-webrtc on Android/iOS captures screen video only; Discord-style
  per-app audio capture would need a native `AudioPlaybackCapture` module.
- Calls across different networks usually need a TURN server; add one to
  `ICE_SERVERS` in `mobile/src/lib/config.ts`.

## API

All JSON. `Authorization: Bearer <token>` after `/api/register` or `/api/login`.
Every route is explained in [docs/routes.md](docs/routes.md).

```
POST   /api/register  /api/login  /api/logout      GET /api/me
PUT    /api/me {username?, display_name?, bio?, pronouns?, avatar_url?, avatar_crop?}   (avatar: a picture uploaded via /api/media, GIFs included;
       avatar_crop {x, y, w, aspect}: the square shown in the circle, as fractions of the picture)
PUT    /api/me/presence {invisible}   appear offline to everyone
PUT    /api/me/password {current, new}   signs out the other devices; after -reset-password it is the only call allowed
GET    /api/users?q=   /api/users/{id}   /api/users/{id}/posts
GET    /api/feed?before=&limit=
POST   /api/posts {body, group_id?}    GET|PUT|DELETE /api/posts/{id}   (PUT: author, {body, attachments})
GET|POST /api/posts/{id}/comments {body, parent_id?}   PUT|DELETE /api/comments/{id} (author)
PUT|DELETE /api/comments/{id}/reaction {emoji}   GET /api/comments/{id}/reactions
PUT|DELETE /api/posts/{id}/reaction {emoji}   GET /api/posts/{id}/reactions (who reacted with what)
POST   /api/media (the file as the body) -> {type, url, width, height}   GET /media/{name}
GET    /api/friends   /api/friends/requests
POST   /api/friends/{id}/request   /api/friends/{id}/accept    DELETE /api/friends/{id}
GET    /api/groups?mine=1|q=       POST /api/groups {name, description, private?, member_ids?}
GET|DELETE /api/groups/{id}        POST /api/groups/{id}/join | /leave
GET    /api/groups/{id}/posts      POST /api/groups/{id}/members {user_ids}   DELETE /api/groups/{id}/members/{uid}
GET|POST /api/groups/{id}/messages {body, attachments}   POST /api/groups/{id}/messages/read   PUT|DELETE /api/groups/{id}/messages/{mid}
PUT|DELETE /api/groups/{id}/messages/{mid}/reaction {emoji}   GET /api/groups/{id}/messages/{mid}/reactions
GET    /api/conversations   (direct chats + group chats)
GET|POST /api/messages/{userId} {body, attachments}
POST   /api/messages/{userId}/read   PUT|DELETE /api/messages/{userId}/{messageId}   (sender; PUT {body})
PUT|DELETE /api/messages/{userId}/{messageId}/reaction {emoji}   GET /api/messages/{userId}/{messageId}/reactions
PUT|DELETE /api/push-token {token}
GET    /api/stats   (disk, uploads and your share, load, memory)
GET    /ws?token=   (every event and client message is a struct in server/events.go, mirrored by
                     mobile/src/lib/events.ts; the call protocol is described in server/calls.go)
```

Server flags: `-addr :8080 -db ami.db -media-dir media`; `-reset-password <username>` (see Forgotten passwords); `-verbose` logs whole requests, responses and WebSocket messages (see Development).
