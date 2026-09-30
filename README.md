# axshare

Live text sharing between machines. Create a room on one laptop, open the
same URL on another, type on either; both stay in sync. One static binary,
one SQLite file, no accounts.

Built as an AthenaX internal tool and as a first Go project.

## Run

```sh
make run              # go run, listens on :8070
make build            # static binary in bin/axshare
./bin/axshare -addr :8070 -db axshare.db
```

Open `http://<host>:8070`, click **create**, send the `/room/<id>` URL to
the other machine.

## How it works

- `net/http` with the Go 1.22 pattern mux; no framework.
- Rooms live in SQLite (`modernc.org/sqlite`, pure Go) behind a small
  `Store` interface; an in-memory implementation backs the tests.
- A live room is one goroutine that owns the client set; clients talk to
  it over channels. Edits are broadcast to every other client and
  persisted; last write wins.
- WebSockets via `coder/websocket`. Graceful shutdown on SIGINT/SIGTERM
  closes rooms, then the HTTP server.
- Rooms expire 24h after their last edit; a ticker sweeps them every minute.
- Per-IP token-bucket rate limit (5 req/s, burst 20) on HTTP endpoints.

```
cmd/axshare        flags, wiring, shutdown
internal/server    routes, handlers, middleware, embedded frontend
internal/hub       rooms, clients, broadcast
internal/store     Store interface, SQLite and memory implementations
```

## API

```
POST /api/rooms          body: initial content → 201 {"id": "..."}
GET  /api/rooms/{id}     → 200 text/plain | 404
GET  /ws/{id}            WebSocket; first frame is current content,
                         every frame sent is broadcast to the room
GET  /healthz            → {"status":"ok","clients":N}
```

## Develop

```sh
make check          # gofmt, vet, tests with -race
make race           # run the server under the race detector
make health         # curl /healthz → {"status":"ok","clients":N}
```

## Known limitations

- Whole-buffer sync, not operational transforms: two people typing in the
  same second will see one another's version win.
- A client that joins a room in the instant its last member leaves is
  closed with "reconnect"; the browser retries.
- No auth. Room IDs are 128-bit random and are the only access control.
  Run it behind HTTPS if it leaves your LAN.