# Docker

Running Ami with Docker, and building it with Docker on a machine that has
neither Go nor Node. The pieces are [`Dockerfile`](../Dockerfile), which
makes one image holding the server and the browser app, and
[`compose.yaml`](../compose.yaml), which runs that image with MariaDB and
coturn. The phone apps are built elsewhere, with the dev shell
([deployment.md](deployment.md#building)), pointed at this server's address.

## Running

Docker Engine 24 or newer with the Compose plugin (`docker compose version`),
on any machine with a public address if people outside your network will use
it. Then:

```sh
git clone https://github.com/anthonychavezdev/Ami.git && cd Ami
cp .env.example .env            # fill in AMI_DB_PASSWORD and AMI_TURN_SECRET: openssl rand -hex 24 for each
docker compose up -d --build    # builds the image (a few minutes the first time), starts everything
docker compose logs -f server   # until "ami listening on :8080", then Ctrl-C
```

Open `http://<the machine>:8080` in a browser, create the first account, and
you are in. What is running:

| Service | Image | Does | Published ports | Keeps data in |
| --- | --- | --- | --- | --- |
| `server` | built from the Dockerfile | The API and the browser app, one origin. | `8080` (`AMI_PORT`) | the `media` volume: uploads |
| `db` | `mariadb:11.4` | The database; reachable from the other containers only. | none | the `db` volume |
| `turn` | `coturn/coturn:4.6` | The TURN relay for calls between networks that cannot reach each other ([calls.md](calls.md#get-apiice)). | `3478` TCP and UDP (`AMI_TURN_PORT`), `49160`-`49200` UDP | nothing |

Open 8080 (or the proxy's 443, see below), 3478 and 49160 to 49200 on the
firewall; nothing else needs to be reachable.

### Production or development

The stack above is production mode: an empty database, registration open
until the first invite code, browser requests accepted from the page's own
origin only. For a look around with data in it, or to work on the app
against these containers, run it in development mode instead. One line in
`.env`:

```sh
AMI_DEV=1
```

then `docker compose up -d` (or `up -d --build` the first time). The server
starts with `-dev`: it seeds the demo accounts (`ana`, `luis`, `sofia`,
`diego` and friends, with the password a dev build's login screen shows and
[development.md](development.md#demo-data) lists), their friendships,
groups, posts, chats and calls into the empty database, and it accepts
browser requests from any origin, so the app served by Metro on port 8081
(`web` or `mobile` in the dev shell, with `AMI_PORT` pointing at the
container's port) can talk to it. The seed runs only when the database is
empty: to start over, `docker compose down -v` and up again. Everything
else, coturn included, runs the same in both modes.

Development mode on a database with real accounts is a mistake: the demo
accounts would be created on any empty one and every website could call the
API with a user's browser. Remove the line and `docker compose up -d` to go
back; the containers are recreated and the data stays.

### Before handing out the address

- **Registration.** It is open to anyone who finds the address until a
  one-time invite code exists. Make one per person:
  `docker compose exec server ami-server -new-invite` prints a code, the
  server needs no restart, and the first code closes open registration for
  good ([server.md](server.md#invite-codes)). A standing code in `.env`
  (`AMI_INVITE`) works as well.
- **Storage.** `AMI_USER_STORAGE_GB` in `.env` caps what one account may
  upload in total; without it the only limit is the disk.
- **TLS.** Browsers allow the camera and the microphone only on HTTPS. Put a
  reverse proxy that terminates TLS in front of port 8080 and tell the server
  to trust it; with [Caddy](https://caddyserver.com) that is a two-line
  `Caddyfile` and automatic certificates:

  ```
  ami.example.com {
      reverse_proxy localhost:8080
      request_body { max_size 20GB }
  }
  ```

  and a `compose.override.yaml` next to `compose.yaml` that adds the flag
  (Compose merges it automatically; the command repeats the Dockerfile's
  `CMD` plus the extra flag):

  ```yaml
  services:
    server:
      command: ["-addr", ":8080", "-media-dir", "/data/media", "-web-dir", "/web", "-trust-proxy"]
  ```

  The same override is where `-allowed-origins` belongs if the page is ever
  served from another origin.

### Settings

Everything is in `.env` ([`.env.example`](../.env.example) lists it):

| Variable | Default | Meaning |
| --- | --- | --- |
| `AMI_DB_PASSWORD` | required | The database user's password, created on the first start. |
| `AMI_TURN_SECRET` | required | Shared by coturn and the server, which mints call credentials from it. |
| `AMI_INVITE` | empty | A standing invite code ([server.md](server.md#invite-codes)). |
| `AMI_USER_STORAGE_GB` | `0` (no limit) | The most one account may upload in total. |
| `AMI_PUBLIC_IP` | detected | The machine's public address, for coturn behind Docker's NAT, when it cannot detect it itself (no route to the internet from the container). |
| `AMI_PORT`, `AMI_TURN_PORT` | `8080`, `3478` | The ports published on the machine. |

Changing `.env` takes effect with `docker compose up -d` (the containers are
recreated).

### Day to day

| Task | Command |
| --- | --- |
| Logs | `docker compose logs -f server` (also `db`, `turn`) |
| Update to the latest code | `git pull && docker compose up -d --build` (the server migrates the database on start, [deployment.md](deployment.md#upgrading)) |
| Invite someone | `docker compose exec server ami-server -new-invite` |
| Reset a forgotten password | `docker compose exec server ami-server -reset-password <username>` ([passwords.md](passwords.md)) |
| Back up the database | `docker compose exec db mariadb-dump --single-transaction -u ami -p"$AMI_DB_PASSWORD" ami > ami-$(date +%F).sql` |
| Back up the uploads | `docker run --rm -v ami_media:/m -v "$PWD":/out alpine tar czf /out/media-$(date +%F).tgz -C /m .` |
| Restore | `docker compose exec -T db mariadb -u ami -p"$AMI_DB_PASSWORD" ami < ami.sql`, and the tarball back into the volume the same way |
| Stop | `docker compose down` keeps the volumes; `docker compose down -v` deletes the database and the uploads |

The volumes are named after the directory the compose file is in (`ami_db`,
`ami_media` for a checkout called `ami`); `docker volume ls` shows them.

### How the containers are locked down

Every service refuses privilege escalation, has a process limit and capped
logs. The server runs as its own user on a read-only root with every Linux
capability dropped; it can write only to the media volume. The database is
not published; its root password is random and used by nobody. coturn relays
only to public addresses, never into the machine's own networks, with quotas
per account and in total ([security.md](security.md)).

## Building

The Dockerfile has four stages: the browser export with Node, the static
binary with Go, an `artifacts` stage holding just those two files, and the
`runtime` image. `docker compose build` and `docker build` make the runtime
image; `--target artifacts` stops before it.

| Want | Command | Result |
| --- | --- | --- |
| The image compose runs | `docker build -t ami-server .` | `ami-server` in the local image store (what `docker compose up --build` does). |
| The server binary and the web build, to run outside Docker | `docker build --target artifacts -o dist .` | `dist/ami-server` and `dist/web/`, the same as the dev shell's `build server` and `build web` ([deployment.md](deployment.md#running-the-server) for running them). |
| Either, for another CPU | add `--platform linux/arm64` or `--platform linux/amd64` | The Node and Go stages run natively on the build machine and cross-compile, so this is as fast as a native build. |
| Push the image to a registry | `docker build -t registry.example.com/ami-server:1.0 . && docker push registry.example.com/ami-server:1.0` | Then `image:` under `server` in `compose.yaml` on the host, and no `build:`. |

Only `server/` and `mobile/` enter the build context
([`.dockerignore`](../.dockerignore)); nothing from the dev shell's caches
or from `dist/` does. The web stage installs packages with
`npm ci --ignore-scripts`, and the Go stage builds with the toolchain
`go.mod` names, so the image gets the standard-library fixes of the newest
patch release ([testing.md](testing.md#dependency-checks)).
