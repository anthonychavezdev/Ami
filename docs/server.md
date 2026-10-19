# The server

The server is the Go program in [`server/`](../server). It is one binary with no
runtime dependencies beyond the SQLite file and the media directory it is pointed
at. This page explains how it is put together; the pages linked from
[routes.md](routes.md) explain what each route does.

## Starting up

`main()` in [`main.go`](../server/main.go) runs in this order:

1. `parseFlags()` reads the command line into a `config` and `verbose` is set from it.
2. If `-reset-password <username>` was given, `printTemporaryPassword` opens the
   database, resets that account ([passwords.md](passwords.md#resetting-a-forgotten-password)) and exits.
3. `signal.NotifyContext` makes a context that ends on Ctrl-C or SIGTERM.
4. `newServer(ctx, cfg)` opens the database (running migrations), creates the
   media directory, builds the limiters and the allowed-origin set, seeds demo data
   when `-dev` and the database is empty, wires presence decoration into JSON
   output and into the hub, configures the WebSocket upgrader, and starts the
   hourly session purge goroutine.
5. `serve(ctx, cfg, s)` builds the HTTP handler and runs `http.Server` until it
   fails or the context ends. Then it stops accepting connections, gives
   in-flight requests `shutdownGrace` (10 s) to finish, and closes every
   WebSocket (`hub.closeAll`, which `Shutdown` cannot do because those
   connections are hijacked).

### Flags

| Flag | Default | Meaning |
| --- | --- | --- |
| `-addr` | `:8080` | Listen address, on every interface. |
| `-db` | `ami.db` | Path of the SQLite database. Created and migrated on start. |
| `-media-dir` | `media` | Directory for uploaded pictures and videos. Created on start. |
| `-dev` | off | Development mode: seed demo data into an empty database and allow any browser origin. |
| `-allowed-origins` | empty | Comma-separated browser origins allowed by CORS and the WebSocket upgrade. Empty means no cross-origin browser access (the page's own origin is always allowed). |
| `-trust-proxy` | off | Take the client IP from `X-Forwarded-For` and treat `X-Forwarded-Proto: https` as TLS. Only behind a reverse proxy you control. |
| `-tls-cert`, `-tls-key` | empty | Serve HTTPS directly from these files. |
| `-web-dir` | empty | Serve the exported browser build from this directory (see [deployment.md](deployment.md#serving-the-web-app)). |
| `-api-rate` | `600` | Requests per minute per client IP on `/api/`; `0` disables. |
| `-verbose` | off | Log every request and response with headers and bodies, and every WebSocket message. See [Logging](#logging). |
| `-reset-password` | empty | A username: give it a temporary password, print it, sign out its devices, exit. |

The `http.Server` is configured with `MaxHeaderBytes` 64 KB, `ReadHeaderTimeout`
10 s, `ReadTimeout` 30 s, `WriteTimeout` 60 s, `IdleTimeout` 2 min and TLS 1.2 as
the minimum version.

### The `server` struct

```go
type server struct {
    db            *sql.DB
    hub           *hub        // live WebSocket connections and presence
    limiter       *limiter    // login/register attempts per IP (10 per minute)
    apiLimiter    *limiter    // every /api request per IP (-api-rate per minute, nil when off)
    uploadLimiter *limiter    // uploads per user (30 per minute)
    origins       map[string]bool
    upgrader      websocket.Upgrader
    mediaDir      string
    trustProxy    bool
    calls         *callRooms  // who is in which call
}
```

Handlers are methods on `*server`, so every one of them can reach the database,
the hub and the call rooms without globals. The two package-level variables that
exist (`presenceHook` in `util.go` and `verbose` in `verbose.go`) are set once at
start.

## The middleware chain

`handler()` in `main.go` wraps the router like this, outermost first:

```
secureHeaders → cors → logRequests (or logTraffic with -verbose) → apiLimit → mux
```

| Layer | File | What it does |
| --- | --- | --- |
| `secureHeaders` | `main.go` | Sets `X-Content-Type-Options: nosniff`, `Cache-Control: no-store`, `Referrer-Policy: no-referrer`, `X-Frame-Options: DENY`, a `default-src 'none'` Content Security Policy, a `Permissions-Policy` that denies camera, microphone, display capture and geolocation, `Cross-Origin-Opener-Policy: same-origin`, and HSTS when the request came over TLS (or through a trusted proxy that says so). The web build and media handlers overwrite the CSP and permissions for their own needs. |
| `cors` | `main.go` | Adds `Vary: Origin`. Requests without an `Origin` (the native app, curl) pass untouched. An allowed origin gets `Access-Control-Allow-Origin` set to itself, the allowed methods and headers, a 600 s preflight cache, and preflights are answered 204. Any other origin gets no CORS headers (the browser blocks the response) and 403 on preflight. `originAllowed` is shared with the WebSocket upgrade. |
| `logRequests` | `main.go` | One line per request: method, path, status, duration. Replaced by `logTraffic` under `-verbose`. |
| `apiLimit` | `main.go` | 429 with `Retry-After: 60` once a client IP exceeds `-api-rate` requests per minute on `/api/` paths (OPTIONS excluded). |
| `mux` | `main.go` (`routes`) | The route table, plus the web build fallback at `/` when `-web-dir` is set. |

Inside the router, individual routes add their own wrappers: `throttle` on
register and login (10 per minute per IP), `auth` on everything that needs a
session, and `directChat`/`groupChat` on the chat routes.

## Routing

Routes are registered with Go's pattern syntax, `mux.HandleFunc("GET /api/users/{id}", ...)`.
`pathID(r, "id")` checks that a path value is a UUID in its canonical form or
returns a 400; `queryID` does the same for query parameters.
Query parameters come through `queryInt` and `pagination`:

- `?before=<id>&limit=<n>` is the cursor for every paginated list: items older
  than the one whose id is `before` (ids are time-ordered UUIDs, so "older" is
  "smaller"), newest first, at most `limit` (default 30, maximum 100; chats
  default to 50). The app passes the id of the last item it has to load the next page.
- Comments use `?after=<id>` instead, oldest first, up to 500.

## Handler conventions

Every handler has one of two shapes:

```go
type handler       func(w http.ResponseWriter, r *http.Request) error
type authedHandler func(w http.ResponseWriter, r *http.Request, me *User) error
```

`public(h)` adapts the first for routes without a session; `auth(h)` adapts the
second after resolving the session token ([login.md](login.md#the-auth-wrapper)).
Both end in `respond(w, err)`:

- `nil`: the handler already wrote its response through `ok` (200), `created`
  (201), `noContent` (204) or `writeJSON`.
- an `*httpError` (made with `fail`, `badRequest`, `forbidden`, `notFound`,
  `conflict`, or `orNotFound` which maps `sql.ErrNoRows` to a 404): that status
  with `{"error": "<message>"}`.
- anything else: logged as `error: ...` and answered `500 {"error": "internal error"}`,
  so database and programming errors never reach clients.

Middleware that must refuse a request before any handler runs uses `writeError`.

Reading input is `readJSON(w, r, &in)`: the body is capped at 1 MB and any decode
failure is a 400 "invalid JSON body". Writing output is `writeJSON`, which first
runs the presence hook over the value (see below), sets the content type and
encodes.

Validation lives next to each handler as small functions that return the cleaned
value and an error (`validCredentials`, `cleanPostInput`, `cleanMessage`,
`validEmoji`, `newUsername`...). Limits are listed per route on each subject page
and gathered in [security.md](security.md#limits).

## Database access

`openDB` in [`db.go`](../server/db.go) opens the file with WAL journaling, foreign
keys on and a 5 s busy timeout, and limits the pool to **one connection**:
SQLite serializes writers anyway, and a single connection means no
`SQLITE_BUSY` errors to handle. Everything else is plain `database/sql`:

- `queryAll(db, scan, query, args...)` runs a query and collects rows through a
  `scan` function, returning a never-nil slice. `scanUser`, `scanPost`,
  `scanMessage`, `scanGroup` and friends are those functions.
- Column lists and scan targets are kept together: `userCols` selects a `User`
  from `users u` and `(*User).fields()` returns the matching pointers in the same
  order. When a row spans several types, `fields(a.fields(), b.fields())` joins the
  lists. Adding a user column means touching both, in the same position, plus the schema.
- Ids are made in Go with `newID()` (a version 7 UUID) before the `INSERT`,
  never by the database ([database-schema.md](database-schema.md#conventions)).
- `queryIDs` collects a single id column; a failing query reads as empty,
  which is right for audience lists.
- `inTx(fn)` runs `fn` in a transaction and commits when it returns nil; a
  rollback is deferred for every other path.
- Timestamps are `nowString()`: RFC 3339 with nanoseconds, UTC. They are stored as
  text and compared as text.
- JSON columns are written with `jsonString(v)` and read with
  `decodeAttachments` (always a slice, never nil) or a `Scan` method
  (`Crop`).

The schema, the migration mechanism and every table are described in
[database-schema.md](database-schema.md).

## Presence in every response

Every `User` object the API returns carries `online`. No handler sets it:
`markPresence` in [`presence.go`](../server/presence.go) is installed as
`presenceHook` and as the hub's `decorate` function, and both `writeJSON` and the
hub's `marshal` pass values through it right before encoding. It walks the value
with reflection, through pointers, slices, maps and nested structs, and sets
`Online` on each `User` from `hub.present(id)`. A non-addressable value is copied
first. The cost is a reflective walk per response, which is nothing next to the
SQL that produced it, and the payoff is that presence can never be forgotten.

## Rate limiting

[`ratelimit.go`](../server/ratelimit.go) is a fixed-window counter per key: the
first hit starts a window, hits inside it count, and the key is allowed while its
count is at or below the maximum. The map is swept of expired windows once it
holds more than 4096 keys. `clientIP` is the peer address, or the first
`X-Forwarded-For` hop when `-trust-proxy` is on.

Three limiters exist: auth attempts (10/min/IP), API requests (`-api-rate`/min/IP)
and uploads (30/min/user). The WebSocket has its own flood guard (300 messages
per 10 s per connection) in `handleWS`.

## Logging

The server logs with the standard `log` package to stderr, one line per request
(`GET /api/feed 200 3ms`), plus migrations, call messages, push failures and
internal errors. With `-verbose` ([`verbose.go`](../server/verbose.go)) the request
line becomes a numbered entry with the client address, every header and the
body, followed by the response with its status, duration, the session's user,
headers and body; WebSocket messages in and out and push traffic are logged too.
Passwords and tokens are redacted (the credential fields of login, register,
password change and push-token calls, the `Authorization` header and the
WebSocket's `?token=`), uploads are shown by type and size only, and bodies are
cut at 4 KB. See [development.md](development.md#watching-the-traffic).

## Shutting down

On Ctrl-C or SIGTERM the context from `signal.NotifyContext` ends. `serve` calls
`srv.Shutdown` with a 10 s grace, then `hub.closeAll` closes every WebSocket so
each `handleWS` runs its cleanup (leaving calls, announcing presence). The
session purge loop and anything else watching the context stop on their own.
Requests still running after the grace are cut off; `serve` treats that as a
clean exit.

## Tests

The suite in `server/*_test.go` starts a real server on a temporary database for
every test and drives it over HTTP and WebSocket. [testing.md](testing.md)
describes the helpers and what each test covers.
