# Ami documentation

Ami is a small social network: posts with comments and reactions, friends,
groups, direct and group chats, and peer-to-peer voice and video calls with
screen sharing. It is one Go binary with a SQLite file behind it and one
Expo (React Native) app that runs on Android, iOS and in the browser.

These pages explain how everything works, from the first request to the last
pixel, so that someone new can pick the project up and change it with
confidence. Every page links to the source it describes; paths are relative to
the repository root, so `server/hub.go` is `../server/hub.go` from here.

## Start here

1. [Overview](overview.md): the two halves, how a request and an event travel, and a map of the code.
2. [Development](development.md): the dev shell, the scripts, the ports, the daily loop and its traps.
3. [Routes](routes.md): every HTTP route and WebSocket message, each linked to its detailed page.
4. [Database schema](database-schema.md): the tables, a diagram, and how migrations work.

## By subject

| Page | What it covers |
| --- | --- |
| [server.md](server.md) | How the Go server is put together: flags, startup, middleware, handler conventions, helpers, shutdown. |
| [mobile-app.md](mobile-app.md) | How the app is put together: routing, state, networking, the wide layout, components and platform files. |
| [login.md](login.md) | Accounts and sessions: register, login, logout, tokens, the forced-password gate. |
| [passwords.md](passwords.md) | Changing a password, the `-reset-password` recovery flow. |
| [profile.md](profile.md) | Editing your profile, profile pictures and their crop, appearing offline. |
| [users.md](users.md) | Searching people, viewing a profile, a person's posts. |
| [posts.md](posts.md) | The feed, writing and editing posts, reactions, live updates. |
| [comments.md](comments.md) | Threaded comments, editing, deleting with placeholders. |
| [media.md](media.md) | Uploads, how files are checked and served, attachments on posts and messages. |
| [friends.md](friends.md) | Friend requests, the friendship states, what friendship unlocks. |
| [groups.md](groups.md) | Groups, roles, invite-only groups, membership. |
| [messages.md](messages.md) | Direct and group chats, unread counts, delivery. |
| [notifications.md](notifications.md) | OS notifications, badges and push through Expo. |
| [websocket.md](websocket.md) | The realtime connection, the hub, presence, every event. |
| [calls.md](calls.md) | Calls: rooms, the signaling protocol, the client call manager, screen sharing. |
| [security.md](security.md) | Every security control, and what is deliberately not there. |
| [testing.md](testing.md) | The Go suite, the app checks and the end-to-end scripts. |
| [deployment.md](deployment.md) | Building, running in production, TLS, backups, operations. |
| [dependencies.md](dependencies.md) | Every dependency and toolchain piece: what it does here and why it was chosen. |

## Conventions in these pages

- "The server" is the Go program in `server/`; "the app" is the Expo project in `mobile/`.
- Routes are written as `METHOD /path`. Request and response bodies are JSON unless the page says otherwise.
- Code references name the file and, where useful, the function: `server/sessions.go` (`auth`).
- Diagrams are Mermaid; GitHub renders them, and most editors have a preview.
