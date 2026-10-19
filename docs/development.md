# Development

Everything about working on Ami day to day: the shell, the scripts, the ports,
the loops for each platform, and the traps.

## The shell

The repository is a [devenv](https://devenv.sh) project ([`devenv.nix`](../devenv.nix)):
`devenv shell` (or `direnv allow` once, through [`.envrc`](../.envrc)) gives you
Go, Node and npm, the Android SDK with an emulator, JDK 17, watchman, the
`sqlite3` CLI, Python for the end-to-end scripts and, on macOS, CocoaPods.
Nothing is installed globally: Go's module cache, npm's cache, Gradle, CocoaPods
and the Android AVDs all live inside the repository (`.go/`, `.npm/`, `.gradle/`,
`.cocoapods/`, `.android/`), all gitignored. Xcode is the one thing Nix cannot
provide; install it from the App Store for iOS.

The Android SDK ships under Google's license, which `devenv.yaml` accepts with
`allow_unfree: true`.

## Scripts

| Command | What it does |
| --- | --- |
| `setup` | `go mod tidy` in `server/` and `npm install` in `mobile/`. Run once, and after dependency changes. |
| `server [flags]` | The API with the production defaults on `:$AMI_PORT`. `server -dev` seeds demo data and allows any browser origin; `server -dev -verbose` also logs every request, response and socket message. Flags go straight to the binary. |
| `devenv up` | The same server in dev mode under process-compose, for people who prefer a supervised process. |
| `web` | Metro serving the app as a web page on `http://localhost:$AMI_METRO_PORT`. |
| `mobile` | Metro alone, for an app already installed on a device or emulator. |
| `android` | Creates the `ami` emulator if needed (Pixel 7, API 36), builds the development build, installs it on the emulator or a USB phone, starts Metro. |
| `ios` | Same for the iOS Simulator (macOS with Xcode). |
| `test` | gofmt, `go vet`, `go test ./...`, then `tsc`, ESLint and Prettier. Everything that must pass before a commit. |
| `build [server\|web\|android\|ios]` | Release artifacts into `dist/` ([deployment.md](deployment.md)). |

## Ports and addresses

| Variable | Default | Meaning |
| --- | --- | --- |
| `AMI_PORT` | `8080` | The API's port, and where the app looks for it. |
| `AMI_METRO_PORT` | `8081` | Metro, which also serves the web page. Native dev builds bake it in: rebuild after changing it. |
| `AMI_HOST` | LAN IP | The address phones use to reach this machine, when Expo's own detection picks the wrong interface (VPNs, several networks). |
| `EXPO_PUBLIC_API_URL` | empty | The full API URL when the server runs elsewhere; baked into native builds. |

Override them in a gitignored `.env` at the repository root (`NAME=value` per
line, loaded when entering the shell), in `devenv.local.nix`, or for one
command (`AMI_PORT=9000 server`). The shell exports `EXPO_PUBLIC_API_PORT` from
`AMI_PORT` for the app, and `REACT_NATIVE_PACKAGER_HOSTNAME` from `AMI_HOST`
for Metro. How the app resolves the API address from these is in
[mobile-app.md](mobile-app.md#where-the-api-is).

## Demo data

`server -dev` fills an empty database with eight accounts (`ana`, `luis`,
`sofia`, `diego`, `valentina`, `mateo`, `camila`, `javier`, all with the
password `amigo-demo`), friendships in every state, four groups (one invite
only), posts with pictures, links and Markdown, comments, reactions, direct and
group chats, and unread badges for `ana`. Log in as `ana` to see every state.
The content is defined in [`server/demo.go`](../server/demo.go) and inserted by
[`server/seed.go`](../server/seed.go); the demo pictures are gradients generated
into the media directory. To start over, stop the server and delete
`server/ami.db*`.

## The daily loop

### Browser

```sh
server -dev        # terminal 1
web                # terminal 2, opens http://localhost:8081
```

Metro rebuilds on save and the page refreshes. `-dev` matters here: the page on
port 8081 calls the API on port 8080, which is a different origin, and only
`-dev` (or `-allowed-origins`) lets it through CORS. Browsers only expose the
camera and microphone on `localhost` or HTTPS, so calls work on this machine
but not from another machine over plain HTTP.

### Android

```sh
server -dev
android            # first time: ~10 minutes of Gradle; then installs and starts Metro
```

From then on `mobile` is enough: the installed build connects to Metro and
reloads on save. Rebuild (`android` again) after changing native dependencies,
`app.json` or a plugin, after running `npx expo prebuild --clean`. The emulator
is `ami`; a USB phone takes precedence when plugged in. On Linux the emulator
needs `/dev/kvm` access (add yourself to the `kvm` group); if `android` reports
that no emulator could be started, run `emulator -avd ami` by hand to see the
real error.

### iOS

```sh
server -dev
ios                # Xcode from the App Store, signed in; the Simulator opens
```

The script runs Xcode's tools in a minimal environment because the Nix toolchain
in `PATH` and `DEVELOPER_DIR` would break the link step.

### Two users at once

Open a second browser in a private window, a second emulator
(`avdmanager create avd -n ami2 -d pixel_7 -k <system image>` then
`emulator -avd ami2`), or drive the second user through the API with `curl`
(the demo password is in `demo.go`). Presence, messages, friend changes and
calls all show up live on the other side.

## Watching the traffic

`server -dev -verbose` logs every request and response with their headers and
bodies, every WebSocket message in and out with who received it, and push
traffic, with passwords and tokens redacted and bodies cut at 4 KB. It is the
fastest way to see what a client actually sent, and the output is safe to paste
into an issue. See [server.md](server.md#logging).

`sqlite3 server/ami.db` opens the database in the shell; `.tables`, `.schema
posts` and plain SQL work as usual ([database-schema.md](database-schema.md#working-with-the-file)).

## Traps

- **Metro under `CI=1` does not watch files.** Expo disables watching and fast
  refresh when `CI` is set (it prints "Metro is running in CI mode"). Scripts
  and agents that run `CI=1 devenv shell -- web` get one fresh bundle and then
  stale ones; restart Metro with `web -- --clear` after editing, or run it from
  an interactive shell where `CI` is unset.
- **Prebuild after native changes.** `app.json`, `mobile/plugins/` and native
  dependencies only take effect after `npx expo prebuild --clean` and a rebuild.
- **The web page needs `-dev` or `-allowed-origins`** on the server, or every
  request fails CORS.
- **A port in use.** A server or Metro left running from an earlier session
  holds the port; find it with `lsof -i :8080 -sTCP:LISTEN` and stop that
  process, rather than killing everything that mentions the port (the emulator's
  own connections match a looser `lsof`).
- **Linux emulator crashes at start** with a `libabseil_dll.so: undefined symbol`
  error when an old libc++ from the SDK's build tools is on `LD_LIBRARY_PATH`;
  the shell strips those entries ([`devenv.nix`](../devenv.nix), `enterShell`).
- **Session tokens expire after 30 days.** A device that has not been used for a
  month lands on the login screen; that is expected.

## Changing things

| To change... | Touch |
| --- | --- |
| the schema | `schema` and `migrations` in `server/db.go` ([database-schema.md](database-schema.md#adding-a-migration)). |
| a user column | `userCols`, `User.fields()` and the schema on the server; `User` in `mobile/src/lib/types.ts`. |
| a route | `routes()` in `server/main.go`, a handler in the matching file, a function on `api` in `mobile/src/lib/api.ts`, a test in `server/api_test.go`, and the table in [routes.md](routes.md). |
| a realtime event | a struct in `server/events.go`, the union in `mobile/src/lib/events.ts`, a `socket.on` where it matters, a check in `TestRealtime`. |
| a screen | [mobile-app.md](mobile-app.md#adding-a-screen). |
| a dependency | [dependencies.md](dependencies.md#adding-or-upgrading). |
| the demo data | `server/demo.go`; delete the dev database to see it. |

Before committing, run `test`. Commits are small and say what changed and why.
