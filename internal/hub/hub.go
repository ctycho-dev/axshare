// Package hub fans messages out between the WebSocket clients of a paste.
//
// Ownership model: each room has exactly one goroutine (run) that owns the
// client set. Nothing else reads or writes that map. Other goroutines ask
// for changes by sending on the room's channels. That is why there is no
// mutex around clients: a single owner needs no lock.
package hub

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"

	"github.com/ctycho-dev/axshare/internal/store"
)

// message is what travels on room.broadcast. from lets run() skip echoing
// a client's own edit back to it, which would fight the cursor.
type message struct {
	from *Client
	data []byte
}

// room is one paste's live session.
type room struct {
	id         string
	clients    map[*Client]bool
	register   chan *Client
	unregister chan *Client
	broadcast  chan message
	stop       chan struct{} // closed by Hub.Close; run() drops everyone and exits
	store      store.Store
	hub        *Hub
}

// Hub owns the set of rooms. Rooms are created rarely and looked up by
// string, so a mutex is the right tool here; the per-room traffic is what
// goes through channels.
type Hub struct {
	mu     sync.Mutex
	rooms  map[string]*room
	store  store.Store
	active atomic.Int64 // connected clients across all rooms; read by /healthz
}

func New(st store.Store) *Hub {
	return &Hub{rooms: make(map[string]*room), store: st}
}

// ActiveClients is safe to call from any goroutine.
func (h *Hub) ActiveClients() int {
	return int(h.active.Load())
}

// Close stops every room. Each room's run() sees its stop channel close,
// drops its clients (closing their sockets), and returns.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for id, r := range h.rooms {
		close(r.stop)
		delete(h.rooms, id)
	}
}

// room returns the room for id, starting its goroutine on first use.
func (h *Hub) room(id string) *room {
	h.mu.Lock()
	defer h.mu.Unlock()

	if r, ok := h.rooms[id]; ok {
		return r
	}
	r := &room{
		id:         id,
		clients:    make(map[*Client]bool),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan message, 16),
		stop:       make(chan struct{}),
		store:      h.store,
		hub:        h,
	}
	h.rooms[id] = r
	go r.run()
	return r
}

// forget removes an empty room from the index so its goroutine can exit.
func (h *Hub) forget(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.rooms, id)
}

// Join attaches conn to the room for id and blocks until the connection
// closes. The caller's goroutine becomes the read side; the write side is
// spawned here. initial is the paste's current content, sent to the new
// client before anything else.
func (h *Hub) Join(ctx context.Context, id string, conn *websocket.Conn, initial []byte) {
	r := h.room(id)
	c := &Client{
		conn: conn,
		send: make(chan []byte, 8),
		room: r,
	}

	// Queue the initial content before the write pump starts, so it is the
	// first frame the browser sees. Buffered channel, so this cannot block.
	c.send <- initial

	// Known edge: if this room's goroutine exited (last client left) between
	// h.room() above and this send, nobody receives. The timeout turns that
	// rare hang into a quick close; the browser reconnects and gets a fresh
	// room.
	select {
	case r.register <- c:
	case <-time.After(time.Second):
		conn.Close(websocket.StatusTryAgainLater, "room closing, reconnect")
		return
	}

	go c.writePump(ctx)
	c.readPump(ctx) // blocks until the socket closes or ctx ends
	r.unregister <- c
}

// persist writes the latest content back to the store so a room survives
// a restart. Uses a fresh context: the client that sent the edit may be gone.
func (r *room) persist(data []byte) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = r.store.Put(ctx, store.Room{
		ID:        r.id,
		Content:   data,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	})
}

// run is the room's single owner goroutine. It exits when the last client
// leaves or when the hub closes.
func (r *room) run() {
	for {
		select {
		case c := <-r.register:
			r.clients[c] = true
			r.hub.active.Add(1)

		case c := <-r.unregister:
			r.drop(c)
			if r.empty() {
				return
			}

		case m := <-r.broadcast:
			r.persist(m.data)
			for c := range r.clients {
				if c == m.from {
					continue
				}
				select {
				case c.send <- m.data:
				default:
					// Buffer full: this client stopped draining. Drop it
					// instead of letting one dead laptop freeze the room.
					r.drop(c)
				}
			}
			if r.empty() {
				return
			}

		case <-r.stop:
			for c := range r.clients {
				r.drop(c)
			}
			return
		}
	}
}

// empty reports whether the room has no clients and, if so, removes it
// from the hub so the caller can return from run().
func (r *room) empty() bool {
	if len(r.clients) > 0 {
		return false
	}
	r.hub.forget(r.id)
	return true
}

// drop removes c from the room. Closing c.send is what ends its writePump.
// Idempotent: unregister and the slow-client path can both reach here.
func (r *room) drop(c *Client) {
	if !r.clients[c] {
		return
	}
	delete(r.clients, c)
	close(c.send)
	r.hub.active.Add(-1)
}
