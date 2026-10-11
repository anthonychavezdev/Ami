# Ami

Ami (short for *amigo*) is a small social network: posts, comments, emoji reactions, friend requests,
groups, direct messages, and peer-to-peer voice/video calls with screen sharing.

- `server/` — Go backend. Standard library only, plus one dependency:
  `modernc.org/sqlite` (pure-Go SQLite, no cgo). HTTP routing, auth
  (PBKDF2 + session tokens), the WebSocket server (RFC 6455, hand-written) and
  WebRTC signaling are all stdlib.
- `mobile/` — Expo / React Native app (TypeScript, Expo Router, `react-native-webrtc`).
- `devenv.nix` — toolchains and env vars. Everything installs inside this repo
  (`.go/`, `.npm/`, `mobile/node_modules/`, `.gradle/`, `.android/`, `.cocoapods/`).

## Run

```sh
devenv shell          # Go, Node, npm, sqlite3, watchman, Android SDK + emulator, JDK 17
setup                 # go mod tidy + npm install (first time)
server                # API on :8080   (or `devenv up`)
android               # build the app, install it on the emulator (created on first run), start Metro
ios                   # same, in the iOS Simulator (needs Xcode from the App Store)
mobile                # Metro only — once the app is installed, for day-to-day JS work
```

`server` runs with `-dev`, which fills an empty database with demo accounts
(`ana`, `luis`, `sofia`, `diego`, `valentina`, `mateo`, `camila`, `javier`,
password `amigo-demo`) plus friendships, groups, posts and chats. Log in as
`ana` to see every state. Production never seeds.

The app needs a development build (not Expo Go) because of `react-native-webrtc`.
`android` produces one and installs it on the emulator, or on a USB-connected
phone if one is plugged in; `ios` does the same in the Simulator. The native
projects (`mobile/android`, `mobile/ios`) are generated: after changing native
dependencies, `app.json` or `mobile/plugins/`, run `npx expo prebuild --clean`
in `mobile/` so the next `android`/`ios` picks the changes up.

The app finds the server automatically on the machine running Metro (port 8080).
To point it elsewhere: `EXPO_PUBLIC_API_URL=http://192.168.1.10:8080 npx expo start`.

## Production

`build` writes release artifacts to `dist/`; `build server`, `build android` or
`build ios` makes just one:

- `dist/ami-server`: static, stripped binary (set `GOOS`/`GOARCH` to cross-compile).
  Run it with `-db /var/lib/ami/ami.db` and either `-tls-cert`/`-tls-key`, or
  behind a TLS-terminating reverse proxy plus `-trust-proxy`. Never pass `-dev`.
  Other flags: `-addr`, `-allowed-origins` (CORS is off by default).
- `dist/ami-release.apk`: minified release build (Hermes bytecode, R8, resource
  shrinking) signed with your keystore, never the debug key. Required:
  `EXPO_PUBLIC_API_URL=https://…` (baked into the app), `AMI_KEYSTORE`,
  `AMI_KEYSTORE_PASSWORD`; optional `AMI_KEY_ALIAS` (default `ami`).
- `dist/Ami.ipa`: Release archive exported by Xcode with automatic signing.
  Needs Xcode signed in to your Apple Developer account and `AMI_APPLE_TEAM`
  (the 10-character team id). `AMI_IOS_METHOD` picks the export:
  `app-store-connect` (default; upload with Transporter or TestFlight),
  `release-testing` (ad hoc, for registered devices) or `debugging`.

The server stores session tokens hashed, rate-limits login/register per IP and
sends `no-store`/`nosniff` headers. The app keeps its token in SecureStore and
ships no secrets.

## Features

| Area | Notes |
|---|---|
| Posts | Markdown text, pictures, videos and links, together or on their own. Feed = your posts + friends' posts + posts in your groups. Profile posts are public. |
| Reactions | Tap **React** to 👍; press-and-hold opens the emoji menu. One reaction per user per post, any emoji. |
| Comments | On the post screen. |
| Friends | Request → accept. Only friends can message/call each other. Pushed live over WebSocket. |
| Groups | Creator is owner + admin; others join as members. Admin can remove members, delete posts, delete the group. |
| Messages | Direct messages, delivered in real time. |
| Calls | Voice or video, 1:1, WebRTC with STUN (`stun.l.google.com`). Mute, camera on/off, flip camera. |
| Screen share | Toggle during a call; sent as a second video stream. |
| Theme | Light, dark, or follow the device setting (Me tab). |

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

```
POST   /api/register  /api/login  /api/logout      GET /api/me
GET    /api/users?q=   /api/users/{id}   /api/users/{id}/posts
GET    /api/feed?before=&limit=
POST   /api/posts {body, group_id?}    GET|DELETE /api/posts/{id}
GET|POST /api/posts/{id}/comments      PUT|DELETE /api/posts/{id}/reaction {emoji}
POST   /api/media (multipart "file") -> {type, url, width, height}   GET /media/{name}
GET    /api/friends   /api/friends/requests
POST   /api/friends/{id}/request   /api/friends/{id}/accept    DELETE /api/friends/{id}
GET    /api/groups?mine=1|q=       POST /api/groups {name, description}
GET|DELETE /api/groups/{id}        POST /api/groups/{id}/join | /leave
GET    /api/groups/{id}/posts      DELETE /api/groups/{id}/members/{uid}
GET    /api/conversations          GET|POST /api/messages/{userId}
GET    /ws?token=   (events: hello, message, friend.request, friend.accepted,
                     call.offer/answer/ice/streams/hangup/reject/busy/unavailable)
```

Server flags: `-addr :8080 -db ami.db -media-dir media`.
