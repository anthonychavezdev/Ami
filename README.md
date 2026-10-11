# go-social

A small social network: posts, comments, emoji reactions, friend requests,
groups, direct messages, and peer-to-peer voice/video calls with screen sharing.

- `server/` — Go backend. Standard library only, plus one dependency:
  `modernc.org/sqlite` (pure-Go SQLite, no cgo). HTTP routing, auth
  (PBKDF2 + session tokens), the WebSocket server (RFC 6455, hand-written) and
  WebRTC signaling are all stdlib.
- `mobile/` — Expo / React Native app (TypeScript, Expo Router, `react-native-webrtc`).
- `devenv.nix` — toolchains and env vars. Everything installs inside this repo
  (`.go/`, `.npm/`, `mobile/node_modules/`, `.gradle/`, `.cocoapods/`).

## Run

```sh
devenv shell          # Go, Node, npm, sqlite3, watchman
setup                 # go mod tidy + npm install (first time)
server                # API on :8080   (or `devenv up`)
mobile                # Metro / Expo dev server
```

The app finds the server automatically on the machine running Metro (port 8080).
To point it elsewhere: `EXPO_PUBLIC_API_URL=http://192.168.1.10:8080 npx expo start`.
