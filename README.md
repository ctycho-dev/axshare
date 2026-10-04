<p align="center">
  <img src="web/public/favicon.svg" width="72" alt="axshare">
</p>

<h1 align="center">Axshare</h1>

<p align="center">
  Live text sync between machines. Create a file, share the link, edit it from anywhere.<br>
  One static binary, one SQLite file. No account needed: files last 24 hours, or 7 days when you sign in.
</p>

<p align="center">
  <a href="https://axshare.dev"><b>axshare.dev</b></a> ·
  <a href="#run">Run</a> ·
  <a href="#configuration">Configuration</a> ·
  <a href="#how-it-works">How it works</a> ·
  <a href="#api">API</a> ·
  <a href="#develop">Develop</a> ·
  <a href="#deploy">Deploy</a>
</p>

A self-hosted replacement for codefile.io. Open a room on one laptop, open the same URL on another, type on either; both stay in sync over a WebSocket within ~200 ms. Syntax highlighting for the usual suspects (TS, JS, Go, Python, Rust, Markdown, JSON, YAML), with the file type saved per room. Light, dark and auto themes, keyboard-first.

Signing in with GitHub or Google is optional. It gives the files you create an owner, a 7-day lifetime, and a "my files" list that follows you across devices.

Built as a first Go project. Go on the backend, no framework; Vite + vanilla TypeScript + CodeMirror 6 on the front, bundled into the binary with `embed`.

## Run

```sh
make run                      # go run, listens on :8070
make build                    # frontend + static binary → bin/axshare
./bin/axshare -addr :8070 -db axshare.db
```

Open `http://<host>:8070`, click **create file** (or press `n`), send the `/room/<id>` link to the other machine.

Or with Docker:

```sh
docker compose up -d --build  # builds node → go → distroless
```

## Configuration

Flags: `-addr` (default `:8070`), `-db` (default `axshare.db`), `-version`.

Sign-in is configured through environment variables. With none set, the server runs with sign-in disabled and everything else works.

```sh
BASE_URL=http://localhost:8070      # public address; used to build OAuth callback URLs
GITHUB_CLIENT_ID=...
GITHUB_CLIENT_SECRET=...
GOOGLE_CLIENT_ID=...
GOOGLE_CLIENT_SECRET=...
```

Locally, put them in a `.env` file (no quotes around values); the Makefile loads it. In Docker, `compose.yaml` reads the same file through `env_file`. `.env` is listed in both `.gitignore` and `.dockerignore`.

Each provider needs these redirect URIs registered, for every `BASE_URL` you use:

```
<BASE_URL>/auth/github/callback
<BASE_URL>/auth/google/callback
```

## How it works

```
cmd/axshare        flags, env config, wiring, graceful shutdown
internal/server    routes, handlers, auth, middleware, embedded frontend
internal/hub       rooms, clients, broadcast
internal/store     Store interface, SQLite implementation, migrations
web                Vite + TS frontend, built into internal/server/dist
```

- **HTTP** — `net/http` with the Go 1.22 pattern mux. Request logging, a per-IP token-bucket rate limit (5 req/s, burst 20) and session loading are plain `func(http.Handler) http.Handler` middleware. Behind a local reverse proxy the limiter keys on the last `X-Forwarded-For` entry, the one the proxy appended.
- **Storage** — SQLite (`modernc.org/sqlite`, pure Go, no cgo) behind a `Store` interface. Tests run against a real database in a temp directory.
- **Migrations** — numbered SQL files in `internal/store/migrations`, embedded in the binary and applied in order at startup. Each runs in a transaction together with its row in `schema_version`. Forward only: once a migration has run on a live database, never edit it; add a new file.
- **Live sync** — each room is one goroutine that owns its client set. Clients talk to it over channels (`register`, `unregister`, `broadcast`); there is no mutex around the clients map because it has a single owner. Slow clients are dropped with a non-blocking send instead of stalling the room. Whole-buffer sync, last write wins.
- **Sign-in** — OAuth via `golang.org/x/oauth2`. A provider is a small struct (name, OAuth config, a function that fetches the profile), so GitHub and Google share one login handler and one callback handler. A login is stored as `(provider, provider_user_id)` pointing at a user.
- **Sessions** — a random token in an `HttpOnly`, `SameSite=Lax` cookie, valid 30 days. The database stores only the SHA-256 of the token. Middleware looks the session up and puts the user into the request context.
- **Lifecycle** — one `context.Context` from `signal.NotifyContext` unwinds everything on SIGINT/SIGTERM: rooms close their sockets, the HTTP server drains, the DB closes. A room expires 24 h after its last edit, or 7 days if it has an owner; a ticker sweeps expired rooms and sessions every minute.
- **Frontend** — two pages, one stylesheet, no framework. CodeMirror 6 with a theme driven by CSS variables; colors are defined once with `light-dark()` and switched by `color-scheme`. The landing page shows "my files" (from the server, when signed in) and "recent on this machine" (`localStorage`), five rows each.

Room IDs are 128-bit `crypto/rand` and are the only access control: if you have the link, you can read and edit. Owning a room does not restrict who can edit it.

## API

```
POST  /api/rooms               body: initial content → 201 {"id": "…"}
                               a signed-in caller becomes the owner
GET   /api/rooms/{id}          → 200 text/plain | 404
                               headers: X-Room-Ext, X-Room-Expires (Unix s),
                               X-Room-TTL (s)
PATCH /api/rooms/{id}          body: {"ext": "go"} → 204 | 400 | 404
GET   /api/me                  → {"signed_in", "name", "avatar_url", "providers"}
GET   /api/me/rooms            → [{"id", "ext", "bytes", "expires"}] | 401
GET   /auth/{provider}/login   redirects to GitHub or Google
GET   /auth/{provider}/callback  completes sign-in, sets the session cookie
POST  /auth/logout             → 204
GET   /ws/{id}                 WebSocket. First frame is the current content;
                               every frame sent is broadcast to the room.
GET   /healthz                 → {"status":"ok","clients":N}
GET   /room/{id}               the editor page
```

## Develop

```sh
make check          # gofmt, vet, go test -race ./...
make race           # run the server under the race detector
make health         # curl /healthz

make run            # terminal 1: Go on :8070
make dev            # terminal 2: Vite on :5173 with hot reload, proxying /api and /ws
```

Frontend changes are baked in at `go build` time via `//go:embed all:dist`, so after `npm run build` restart the Go process. During development use the Vite server instead. Vite does not proxy `/auth`, so test sign-in against the Go binary.

## Deploy

The production instance is a Docker Compose service on a VPS behind nginx with a certbot certificate. `compose.yaml` binds the port to `127.0.0.1` and keeps the SQLite file in a named volume; the container runs as `nonroot`, so the volume mountpoint needs to be owned by uid 65532 once.

The server needs its own `.env` with `BASE_URL=https://axshare.dev` and the provider credentials. An `https` base URL also marks the cookies `Secure`. After changing `.env`, recreate the container: `docker compose up -d --force-recreate`.

nginx needs the `Upgrade` headers on `/ws/`, a long `proxy_read_timeout`, and `X-Forwarded-For`; a working server block is in `deploy/nginx.conf`. Updating is `git pull && docker compose up -d --build`. Migrations apply on start, so copy the data volume first when a release adds one.

For Google sign-in to accept any account, the OAuth app must be published ("In production"), which requires a homepage and a privacy policy URL; the policy is served at `/privacy.html`.

## Load test

`cmd/loadtest` opens many WebSocket clients against a test instance and measures how long an edit takes to reach the other clients in its room. Each room gets one writer sending an edit per second and one reader.

```sh
ulimit -n 20000                                            # in both terminals
go run ./cmd/axshare -no-ratelimit -addr :8071 -db /tmp/axload.db
go run ./cmd/loadtest -rooms 1000 -clients 2 -duration 30s
```

`-no-ratelimit` exists for this purpose only. Never point the tool at a production instance: it fills it with junk rooms.

Local results, server and load tool on the same machine (<your machine>), 30-second runs, 2 KB edits unless noted:

| Clients | Edits/s | p50 | p95 | p99 | Delivered |
|---|---|---|---|---|---|
| 500 | 241 | 1.1 ms | 4.9 ms | 12.0 ms | 100% |
| 1,000 | 482 | 0.8 ms | 3.9 ms | 9.5 ms | 100% |
| 2,000 | 960 | 0.5 ms | 4.1 ms | 13.2 ms | 100% |
| 5,000 | 2,370 | 0.2 ms | 3.4 ms | 18.5 ms | 100% |
| 500 (100 KB edits) | 241 | 1.6 ms | 6.8 ms | 13.6 ms | 100% |

p95 delivery time stays under 5 ms from 500 to 5,000 concurrent clients. The worst single edit at 5,000 clients took 2.4 s, which is the first sign of strain in the tail.

These are loopback figures: no network, no nginx, no TLS, and every room active every second. They show the Go server is not the bottleneck. In production the limits are the VPS's CPU and disk and nginx's `worker_connections`, which have not been load-tested.

## Known limitations

- Whole-buffer sync, not OT/CRDT. Two people typing in the same second will see one version win.
- A client that joins a room in the instant its last member leaves is closed with "reconnect"; the browser retries.
- A GitHub login and a Google login are separate accounts, even with the same email.
- A room saved as `plain` still gets a content-based language guess on load, so explicitly choosing plain for code-like text does not survive a reload.
- Signing in from a room page returns to the landing page.
- No self-service account deletion yet.
- Planned: sign-in by email code and LinkedIn.

## License

MIT

---

<sub>Built as a Go learning project with Claude: the core written by hand and reviewed, later features pair-written.</sub>