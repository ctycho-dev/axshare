package server

import (
	"errors"
	"log"
	"net/http"

	"github.com/coder/websocket"

	"github.com/ctycho-dev/axshare/internal/store"
)

// handleWS upgrades GET /ws/{id} to a WebSocket and hands it to the hub.
// The handler does not return until the connection closes: Join runs the
// read side on this goroutine, which is what net/http gave us for free.
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	p, err := s.store.Get(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Accept checks the Origin header against the request Host, so a page
	// served from another site cannot open sockets to us. Our page and the
	// socket share an origin, so the default is what we want.
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		// Accept has already written the HTTP error response.
		log.Printf("ws %s: accept: %v", id, err)
		return
	}

	s.hub.Join(r.Context(), id, conn, p.Content)
}
