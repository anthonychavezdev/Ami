# Testing

Three layers: a Go suite that drives a real server, static checks on the app,
and Python scripts that drive the Android build on emulators for what the
first two cannot reach. `test` in the dev shell runs the first two.

## The Go suite

`server/*_test.go` starts a real server for every test: `newTestServer`
([`helpers_test.go`](../server/helpers_test.go)) builds a `server` on a fresh
MariaDB database of its own (`testDatabase` creates `ami_test_<random>` and
drops it when the test ends), wraps it in the real handler chain, and serves
it with `httptest`. The database comes from `AMI_TEST_DSN`, or the devenv
MariaDB over its socket as root; the `test` script starts one for the run
when `devenv up` is not keeping one up, so `test` works on its own, while
a bare `go test` needs the database running. Tests then talk HTTP and WebSocket exactly like the
app, so routes, middleware, events and the database are all exercised together.
`TestMain` lowers PBKDF2 to 1,000 rounds and the presence grace to 300 ms so the
suite runs in a few seconds, and silences the request log unless `AMI_TEST_LOG`
is set, which keeps it for reading a failure's SQL errors.

Helpers worth knowing:

- `J` is a decoded JSON object with forgiving accessors (`str`, `num`, `id`,
  `obj`, `list`, `isTrue`, `isNull`, `has`), so checks read like the JSON they inspect.
- `ts.get/post/put/del` send requests with a token; `ts.upload` posts a
  file as the body; `ts.register`, `ts.login`, `ts.befriend` set up accounts.
- `ts.check(name, cond, info...)` records a failed expectation without stopping
  the test, so one run lists every broken promise.
- `ts.connect(account)` opens a WebSocket and consumes its `hello`; `expect(type)`
  reads until that event arrives (skipping others), `until(pred)` waits for a
  condition, `send` writes a client message.
- `tinyPNG()` and `tinyGIF()` are real encoded pictures for upload tests;
  `fakeExpo` ([`push_test.go`](../server/push_test.go)) stands in for Expo's push API.

What each test covers ([`api_test.go`](../server/api_test.go) unless noted):

| Test | Promises |
| --- | --- |
| `TestHealthAndOrigins` | Health, security headers, CORS for allowed, own and other origins. |
| `TestAuth` | Register validation and session, duplicate usernames in any case, wrong password, demo login, `/api/me`, logout invalidating the token. |
| `TestLoginRateLimit` | The 11th attempt in a minute is 429. |
| `TestSeededContent` | The demo feed, group chats and badges for `ana`, media served immutable, path traversal refused. |
| `TestPosts` | Markdown posts, uploads (sizes, sandbox CSP, per-user budget, disguised files), attachments and links, feed paging and visibility, deletes. |
| `TestProfile` | Every `PUT /api/me` rule, renaming and logging in with the new name, GIF avatars, crops and their validation, interests (and searching by them), links, the Markdown bio, the cover and its crop. |
| `TestCommentsAndReactions` | Comments, replies, nesting, edits, `[deleted]` placeholders and pruning, counts, reactions. |
| `TestFriends` | Search, every request transition, 409s, messaging non-friends, feed after accepting, unfriending, mutual requests. |
| `TestMessages` | Direct messages, history and cursors, attachments, unread counts and reading, push token routes, edits and deletes. |
| `TestRealtime` | `hello`, presence (connect, appear offline, grace), origin and token checks on the upgrade, ping/pong, every message, friend, post and comment event with its fields and audience. |
| `TestCalls` | One-to-one calls end to end: rooms, ringing, joining, relayed offers, answers, ICE and streams, outsiders refused, leaving, decline reasons, offline callees, non-friends, self-calls, unknown rooms. |
| `TestGroups` | Creation, discovery, posting rules, joining, admin powers, owner rules, deletion. |
| `TestGroupChats` | Private group chats among friends, membership gates, group messages and their events, read markers for new members, conversations, admin deletes, group calls with seats surviving reconnects, leaving and deleting. |
| `TestPasswords` | Changing a password, other devices signed out, `-reset-password` end to end including the forced-change gate. |
| `TestServeStopsWhenContextEnds` | Graceful shutdown returns cleanly. |
| `TestMigrateFirstRelease`, `TestNewDatabaseIsCurrent`, `TestMigrateFromVersion1` ([`migrate_test.go`](../server/migrate_test.go)) | A first-release database upgrades through every migration, its integer ids becoming UUIDs with every reference, order and read marker kept; a new database is already current, with text ids; a database taken back to version 1 re-runs the later migrations, which leave its text ids alone. |
| `TestPush` | Offline users are pushed with the badge count, dead tokens are forgotten, online users are not pushed. |
| `TestVerboseLog`, `TestVerboseLogLongCredentials` ([`verbose_test.go`](../server/verbose_test.go)) | The verbose log redacts credentials and hands handlers the whole body. |

Run it from the dev shell:

```sh
cd server && go test ./...            # everything, ~4 s
go test -run TestCalls -v ./...       # one test, with its checks
```

A failing check prints its name and the value it looked at
(`friend request: 409 {"error":"..."}`), which is usually enough to see what broke.

## App checks

`npm test` in `mobile/` runs `tsc --noEmit` (strict TypeScript), `eslint .`
(Expo's rules: React, hooks, imports) and `prettier --check .`. There are no
component unit tests; the hooks are small and the end-to-end scripts cover the
flows that matter.

## Dependency checks

Two scanners, run before a release and after a dependency bump:

```sh
cd server && go run golang.org/x/vuln/cmd/govulncheck@latest ./...
cd mobile && npm audit --omit=dev
```

`govulncheck` reports only vulnerabilities in code paths the server actually
reaches, most of them in the standard library, which is fixed by building with
the newest Go patch release: `toolchain` in `go.mod` names it and the dev shell
lets Go fetch it (`GOTOOLCHAIN=auto`), as does the Docker build.

`npm audit` lists everything in the tree, and most of it is build tooling
(Expo CLI, Metro, config plugins, the Xcode project writer) that never ships
in the app. What does ship and stays flagged: `markdown-it` and `linkify-it`
under `react-native-markdown-display`, with quadratic-time rules on hostile
input, which the server's text caps (5,000 bytes a post, 2,000 a comment,
4,000 a message) keep harmless, and the smart-quotes rule is off; and
`decode-uri-component` under `expo-router`, a slow-path on malformed web
URLs. Upgrading past them means the next major of each, so they are taken
with the next Expo SDK rather than overridden.

## End-to-end scripts

[`mobile/e2e/`](../mobile/e2e) holds Python scripts (standard library only) that
drive the Android development build on emulators through `adb` and
`uiautomator`, against the dev server's demo data. They check what the Go suite
cannot: the screens themselves, WebRTC media, and Android's permission dialogs.

- [`ami.py`](../mobile/e2e/ami.py) is the toolkit: `Dev` wraps one emulator
  (dump the UI tree, `find` text, `tap`, `type_text`, screenshots, accept
  permission dialogs, restart the app, `login_as` a demo account, read pixels to
  tell whether video is rendering); `WS` is a bare WebSocket client good enough
  to join a call as a third participant; `step`/`check`/`report` print one
  PASS/FAIL line per step and exit non-zero on any failure; `api`/`login` call
  the server directly.
- [`group_chat.py`](../mobile/e2e/group_chat.py): `luis` opens the football
  group chat, sends a message, and sees one from `ana` (sent through the API)
  arrive live with her name.
- [`group_call.py`](../mobile/e2e/group_call.py): `luis` and `ana` on two
  emulators start and join a group voice call, `diego` joins over the
  WebSocket, the counts update, screen sharing shows on the other device as a
  stage with real pixels, the camera comes on, and everyone leaves cleanly.
- [`direct_call.py`](../mobile/e2e/direct_call.py): `luis` calls `ana` from her
  profile, she is rung and accepts, he shares his screen (seen as a stage on
  her device), stops, and hangs up; both land back where they were.

Run them from the dev shell with the server (`server -dev`), Metro and the
emulators up:

```sh
android                               # once: builds and installs the app on `ami`
emulator -avd ami2 &                  # the call scripts need a second emulator
python3 mobile/e2e/group_chat.py
python3 mobile/e2e/group_call.py
python3 mobile/e2e/direct_call.py
```

`BASE` points the scripts at another server, `E2E_OUT` is where screenshots go.
The scripts assume the demo data as seeded; reset the database if it has drifted.
