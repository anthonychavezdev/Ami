# Deployment and operations

Building the release artifacts, running the server for real, and the chores
that come with it.

## Building

`build` in the dev shell writes everything to `dist/`; `build server`,
`build web`, `build android` or `build ios` makes one ([`devenv.nix`](../devenv.nix)).

| Artifact | How | Notes |
| --- | --- | --- |
| `dist/ami-server` | `CGO_ENABLED=0 go build -trimpath -ldflags="-s -w"` | A static, stripped binary. Set `GOOS`/`GOARCH` to cross-compile (`GOOS=linux GOARCH=amd64 build server`). The MariaDB driver is pure Go, which is what makes this possible. |
| `dist/web/` | `npx expo export --platform web` | The browser build: minified, hashed bundles and an `index.html`. If `EXPO_PUBLIC_API_URL` is set it is baked in; otherwise the page talks to the origin it is served from. |
| `dist/ami-release.apk` | `expo prebuild`, `gradlew assembleRelease` for `arm64-v8a` and `armeabi-v7a`, `apksigner` | Requires `EXPO_PUBLIC_API_URL=https://...`, `AMI_KEYSTORE` and `AMI_KEYSTORE_PASSWORD` (`AMI_KEY_ALIAS` defaults to `ami`). Hermes bytecode, R8 and resource shrinking are on. Never the debug key. |
| `dist/Ami.ipa` | `expo prebuild`, `xcodebuild archive` and `-exportArchive` with automatic signing | macOS with Xcode signed in; `AMI_APPLE_TEAM` is the team id; `AMI_IOS_METHOD` picks `app-store-connect` (default), `release-testing` or `debugging`. |

## Running the server

```sh
mariadb -e "CREATE DATABASE ami CHARACTER SET utf8mb4; CREATE USER 'ami'@'localhost' IDENTIFIED BY 'a long password'; GRANT ALL ON ami.* TO 'ami'@'localhost'"
AMI_DB='ami:a long password@unix(/run/mysqld/mysqld.sock)/ami' ./ami-server -media-dir /var/lib/ami/media -addr :8080
```

- MariaDB 10.5 or newer (`JSON_OBJECTAGG` and `JSON_SEARCH` are used), with a
  database and a user that owns it. Give the connection string through
  `AMI_DB` rather than `-db`, so the password is not on the command line;
  prefer the socket when the database is on the same machine, or
  `tcp(host:3306)` with `tls=true` across machines. The server creates and
  migrates its tables on start, so back the database up before upgrading
  ([database-schema.md](database-schema.md#migrations)). It waits up to 30 s
  for the database at start, which covers a reboot where both come up at once.
- Run it as its own user with write access to the media directory; the binary
  needs nothing else. The directory is created if missing.
- **Never pass `-dev`** in production: it seeds demo accounts into an empty
  database and allows every browser origin.
- TLS: either `-tls-cert` and `-tls-key` to serve HTTPS directly, or a
  reverse proxy that terminates TLS plus `-trust-proxy`, which makes the server
  read client IPs from `X-Forwarded-For` (for rate limits) and treat
  `X-Forwarded-Proto: https` as secure (for HSTS). Only set it when the proxy is
  the only way to reach the port, or clients can spoof those headers.
- The proxy must pass WebSocket upgrades on `/ws` (`Upgrade` and `Connection`
  headers), allow request bodies of 20 GB for uploads without spooling them to
  its own disk first, and should not buffer the WebSocket. For nginx that is
  `proxy_http_version 1.1`, `proxy_set_header Upgrade $http_upgrade`,
  `proxy_set_header Connection "upgrade"`, `client_max_body_size 20g`,
  `proxy_request_buffering off`, plus the usual `X-Forwarded-For` and
  `X-Forwarded-Proto` headers. nginx's `proxy_read_timeout` and
  `proxy_send_timeout` count between successive chunks, so a moving transfer
  of any size is fine at their defaults.
- Health: `GET /api/health` answers `{"status":"ok"}` without a session.
- Logs go to stderr, one line per request; a supervisor (systemd, process
  managers) captures them. `-verbose` is for debugging sessions, not for
  production logs: it writes every body (credentials redacted) and is slow.
- SIGTERM or Ctrl-C shuts down gracefully within 10 s.

A minimal systemd unit:

```ini
[Unit]
After=mariadb.service

[Service]
User=ami
Environment=AMI_DB=ami:a long password@unix(/run/mysqld/mysqld.sock)/ami
ExecStart=/usr/local/bin/ami-server -media-dir /var/lib/ami/media -addr 127.0.0.1:8080 -trust-proxy -web-dir /var/lib/ami/web
Restart=on-failure
```

## Serving the web app

The simplest deployment serves the browser build from the same binary:
`-web-dir dist/web`. `webApp` in [`server/web.go`](../server/web.go) serves
files that exist (hashed bundles under `/_expo/` and `/assets/` with an
immutable cache header), answers every other path with `index.html` so the
client-side router can handle it (with a CSP that allows only the page's own
scripts, inline styles, blob media, and WebSocket connections to the same host),
and keeps `/api/*` as 404s. The page then calls the origin it was loaded from,
so no CORS configuration is needed. Browsers only allow the camera and
microphone, so calls, on HTTPS or `localhost`.

If the web build is hosted elsewhere (a CDN, another domain), build it with
`EXPO_PUBLIC_API_URL=https://api.example` and run the server with
`-allowed-origins https://app.example`.

## The apps

Native builds bake in `EXPO_PUBLIC_API_URL` (and the Metro port, irrelevant in
release). They are development builds in the Expo sense only in debug; the
release APK and IPA are ordinary store-ready builds. Distribution is yours:
sideload the APK, TestFlight or the App Store for the IPA.

### Push notifications

Push needs an EAS project: `eas init` in `mobile/` adds `extra.eas.projectId`
to `app.json`, `eas credentials` uploads the FCM (Android) and APNs (iOS)
keys. The server needs no configuration: it posts to Expo's public push API.
Without a project id the app never registers a token and messages for closed
apps are simply not pushed ([notifications.md](notifications.md)).

## Backups and data

- Back the database up with `mariadb-dump --single-transaction ami > /backups/ami-$(date +%F).sql`,
  which is consistent while the server runs; restore with `mariadb ami < file.sql`
  into an empty database.
- The media directory holds every upload under random names; back it up with
  the database. Nothing in it is ever deleted by the server, including uploads
  that were never attached to anything, so it only grows.
- Sessions, read markers and push tokens are in the database; nothing lives in
  memory that cannot be rebuilt, except call rooms, which are gone on restart.

## Upgrading

1. Back up the database and media.
2. Replace the binary (and the web build if served from it).
3. Restart. Migrations run before the server listens; a failed migration
   leaves the database at its previous version and the server exits with the
   error in its log. The id migration (version 3) rewrites every table once,
   which takes a moment on a large database and needs room for the copy.

The app and the server should be upgraded together when a release changes the
API or an event; the event contract is checked by the compiler on the app side
and by the test suite on the server side, but not at runtime between versions.

## Recovering an account

`AMI_DB=... ./ami-server -reset-password <username>` prints a
temporary password, signs the account out everywhere, and exits; the running
server does not need to stop ([passwords.md](passwords.md#resetting-a-forgotten-password)).

## Sizing

One process over MariaDB: the server comfortably serves a community of
friends on a small VM, and the database takes concurrent writes in stride. Calls do not load it (media is peer to peer). The parts
that grow with users are the media directory and the per-request presence
reflection, neither of which matters at that scale. Horizontal scaling is not
designed in: the hub, the call rooms and the rate limiters are per process.
