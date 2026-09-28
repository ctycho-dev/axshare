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
	stop       chan struct{}
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
	go r.run() // TODO(stage 6): stop this goroutine when the room empties
	return r
}

func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for id, r := range h.rooms {
		close(r.stop)
		delete(h.rooms, id)
	}
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

	r.register <- c
	go c.writePump(ctx)
	c.readPump(ctx) // blocks until the socket closes or ctx ends
	r.unregister <- c
}

// persist writes the latest content back to the store so a room survives
// a restart. Uses a fresh context: the client that sent the edit may be gone.
func (r *room) persist(data []byte) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = r.store.Put(ctx, store.Paste{
		ID:        r.id,
		Content:   data,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	})
}

func (r *room) run() {
	for {
		select {
		case c := <-r.register:
			r.clients[c] = true
			r.hub.active.Add(1)

		case c := <-r.unregister:
			r.drop(c)

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
		case <-r.stop:
			for c := range r.clients {
				r.drop(c)
			}
			return
		}
	}
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
