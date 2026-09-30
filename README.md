---
<p align="center">
  <img src="web/public/favicon.svg" width="72" alt="axshare">
</p>

<h1 align="center">axshare</h1>

<p align="center">
  Live text sync between machines. Create a file, share the link, edit it from anywhere.<br>
  One static binary, one SQLite file, no accounts, gone in 24 hours.
</p>

<p align="center">
  <a href="https://axshare.dev"><b>axshare.dev</b></a> ·
  <a href="#run">Run</a> ·
  <a href="#how-it-works">How it works</a> ·
  <a href="#api">API</a> ·
  <a href="#develop">Develop</a> ·
  <a href="#deploy">Deploy</a>
</p>
---

A self-hosted replacement for codefile.io. Open a room on one laptop, open the same URL on another, type on either; both stay in sync over a WebSocket within ~200 ms. Syntax highlighting for the usual suspects (TS, JS, Go, Python, Rust, Markdown, JSON, YAML), light and dark, keyboard-first.

Built as a first Go project. Go on the backend, no framework; Vite + vanilla TypeScript + CodeMirror 6 on the front, bundled into the binary with `embed`. ~1,200 lines of Go, ~400 of TypeScript.

## Run

```sh
make run                      # go run, listens on :8070
make build                    # frontend + static binary → bin/axshare
./bin/axshare -addr :8070 -db axshare.db
```

Open `http://<host>:8070`, click **create file** (or press `n`), send the `/room/<id>` link to the other machine.

Or with Docker:

```sh
docker compose up -d --build  # builds node → go → distroless, ~15 MB image
```

## How it works

```
cmd/axshare        flags, wiring, graceful shutdown
internal/server    routes, handlers, middleware, embedded frontend
internal/hub       rooms, clients, broadcast
internal/store     Store interface, SQLite and memory implementations
web                Vite + TS frontend, built into internal/server/dist
```

- **HTTP** — `net/http` with the Go 1.22 pattern mux. Request logging and a per-IP token-bucket rate limit (5 req/s, burst 20) as plain `func(http.Handler) http.Handler` middleware.
- **Storage** — rooms live in SQLite (`modernc.org/sqlite`, pure Go, no cgo) behind a four-method `Store` interface. An in-memory implementation backs the tests; one table-driven suite runs against both.
- **Live sync** — each room is one goroutine that owns its client set. Clients talk to it over channels (`register`, `unregister`, `broadcast`); there is no mutex around the clients map because it has a single owner. Slow clients are dropped with a non-blocking send instead of stalling the room. Whole-buffer sync, last write wins.
- **Lifecycle** — one `context.Context` from `signal.NotifyContext` unwinds everything on SIGINT/SIGTERM: rooms close their sockets, the HTTP server drains, the DB closes. Rooms expire 24 h after their last edit; a ticker sweeps them every minute.
- **Frontend** — two pages, one stylesheet, no framework. CodeMirror 6 with a theme driven by CSS variables so light and dark both work. Language is guessed from content and can be overridden in the header. "Recent on this machine" is `localStorage`; the server has no listing endpoint on purpose.

Room IDs are 128-bit `crypto/rand` and are the only access control: if you have the link, you're in.

## API

```
POST /api/rooms          body: initial content       → 201 {"id": "…"}
GET  /api/rooms/{id}     → 200 text/plain | 404
GET  /ws/{id}            WebSocket. First frame is the current content;
                         every frame sent is broadcast to the room.
GET  /healthz            → {"status":"ok","clients":N}
GET  /room/{id}          the editor page
```

## Develop

```sh
make check          # gofmt, vet, go test -race ./...
make race           # run the server under the race detector
make health         # curl /healthz

make run            # terminal 1: Go on :8070
make dev            # terminal 2: Vite on :5173 with hot reload, proxying /api and /ws
```

Frontend changes are baked in at `go build` time via `//go:embed all:dist`, so after `npm run build` restart the Go process. During development use the Vite server instead.

## Deploy

The production instance is a Docker Compose service on a VPS behind nginx with a certbot certificate. `compose.yaml` binds the port to `127.0.0.1` and keeps the SQLite file in a named volume; the container runs as `nonroot`, so the volume mountpoint needs to be owned by uid 65532 once.

nginx needs the `Upgrade` headers on `/ws/` and a long `proxy_read_timeout`; a working server block is in `deploy/nginx.conf`. Updating is `git pull && docker compose up -d --build`.

## Known limitations

- Whole-buffer sync, not OT/CRDT. Two people typing in the same second will see one version win.
- The rate limiter keys on `RemoteAddr`; behind a reverse proxy that is the proxy's address until `X-Forwarded-For` is honored.
- A client that joins a room in the instant its last member leaves is closed with "reconnect"; the browser retries.
- No auth, no ownership. Planned: migrations, a per-room file extension, and sign-in via Google, GitHub, or email code.

## License

MIT

---

<sub>Written by hand as a Go learning project, with Claude reviewing.</sub>