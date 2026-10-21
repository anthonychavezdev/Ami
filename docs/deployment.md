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

## Docker

[`Dockerfile`](../Dockerfile) and [`compose.yaml`](../compose.yaml) run the
whole thing in containers: MariaDB, coturn and the server, which serves the
browser app from the same origin. The phone apps are still built with
`build android` and `build ios`, pointed at the server's `https://` address.

```sh
cp .env.example .env        # then set AMI_DB_PASSWORD and AMI_TURN_SECRET (openssl rand -hex 24)
docker compose up -d --build
docker compose logs -f server
```

- The image is built in three stages: the web export with Node, the static
  binary with Go, and a small Alpine image with just those two, a user of its
  own and the media directory. Rebuild and restart after a `git pull` with
  the same `up -d --build`; the server migrates the database on start.
- Uploads live in the `media` volume, the database in `db`. Back them up with
  `docker compose exec db mariadb-dump -u ami -p"$AMI_DB_PASSWORD" ami > ami.sql`
  and `docker run --rm -v ami_media:/m -v "$PWD":/out alpine tar czf /out/media.tgz -C /m .`
  ([Backups and data](#backups-and-data)).
- Ports: `8080` for the API and the web app, `3478` TCP and UDP plus
  `49160`-`49200` UDP for coturn. Open the coturn ones on the firewall.
  Behind Docker's NAT coturn must advertise the machine's public address: it
  detects it at start, or takes `AMI_PUBLIC_IP` from `.env`.
- TLS: put a reverse proxy that terminates HTTPS in front of port 8080 and
  add `-trust-proxy` to the server's command (`command:` under `server` in a
  `compose.override.yaml`, repeating the arguments from the Dockerfile's
  `CMD`). The browser only allows camera and microphone on HTTPS.
- Who may join and how much they may upload: `AMI_INVITE` in `.env` makes
  registration need that code (hand it to the people you invite), and
  `AMI_USER_STORAGE_GB` caps what one account may upload in total. Both are
  off by default, which is right for a first look and wrong for a public
  address.
- Demo data for a look around: the same override with `-dev` added, on an
  empty database only; never on a real one.
- `docker compose down` keeps the volumes; `down -v` deletes everything.

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

### TURN

Calls flow directly between the two phones once STUN has told each its public
address, which is enough on most home networks. Between two strict NATs (mobile
carriers, office firewalls) the media must be relayed through a TURN server,
and relaying costs bandwidth, so Ami hands out credentials for your own
[coturn](https://github.com/coturn/coturn) rather than a public one
([calls.md](calls.md#get-apiice)). Run it next to the server and give both the
same secret:

```nix
services.coturn = {
  enable = true;
  realm = "ami";
  use-auth-secret = true;
  static-auth-secret-file = "/run/secrets/turn";   # the same value as AMI_TURN_SECRET
  min-port = 49152;
  max-port = 49999;
  no-multicast-peers = true;
  extraConfig = ''
    # any account may ask for a relay: only to public addresses, and not without limit
    user-quota=32
    total-quota=2000
    denied-peer-ip=0.0.0.0-0.255.255.255
    denied-peer-ip=10.0.0.0-10.255.255.255
    denied-peer-ip=100.64.0.0-100.127.255.255
    denied-peer-ip=127.0.0.0-127.255.255.255
    denied-peer-ip=169.254.0.0-169.254.255.255
    denied-peer-ip=172.16.0.0-172.31.255.255
    denied-peer-ip=192.168.0.0-192.168.255.255
    denied-peer-ip=::1
    denied-peer-ip=fc00::-fdff:ffff:ffff:ffff:ffff:ffff:ffff:ffff
    denied-peer-ip=fe80::-febf:ffff:ffff:ffff:ffff:ffff:ffff:ffff
  '';
};
networking.firewall = {
  allowedTCPPorts = [ 3478 ];
  allowedUDPPorts = [ 3478 ];
  allowedUDPPortRanges = [ { from = 49152; to = 49999; } ];
};
```

With `AMI_TURN_SECRET` set (next to `AMI_DB` in the unit's environment) the
server tells the apps to use `turn:<host>:3478`, where `<host>` is the name
they reached the API by, so nothing else needs configuring. A long random
secret (`openssl rand -hex 32`); the credentials the apps get expire after a
day. Optional: `cert`/`pkey` in coturn and `-turn-urls turns:host:5349` to
also offer TURN over TLS on networks that block UDP, and `-stun-urls
stun:host:3478` to keep STUN on your box instead of Google's. The relay uses
no disk and little memory; a relayed voice call is a few KB/s, a video call a
few hundred.

A minimal systemd unit:

```ini
[Unit]
After=mariadb.service

[Service]
User=ami
Environment=AMI_DB=ami:a long password@unix(/run/mysqld/mysqld.sock)/ami
Environment=AMI_TURN_SECRET=the coturn secret
Environment=AMI_INVITE=a long invite code
Environment=AMI_USER_STORAGE_GB=20
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
