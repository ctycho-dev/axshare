package server

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/ctycho-dev/axshare/internal/store"
)

const (
	maxRoomBytes = 1 << 20 // 1 MiB
	roomTTL      = 24 * time.Hour
)

// handleCreateRoom: POST /api/rooms. Body is the initial content (may be
// empty). Responds 201 with {"id": "..."}.
func (s *Server) handleCreateRoom(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRoomBytes)
	content, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "body too large or unreadable", http.StatusBadRequest)
		return
	}

	id, err := newID()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	now := time.Now()
	room := store.Room{
		ID:        id,
		Content:   content,
		CreatedAt: now,
		ExpiresAt: now.Add(roomTTL),
	}
	if err := s.store.Put(r.Context(), room); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(struct {
		ID string `json:"id"`
	}{id}); err != nil {
		log.Printf("create room: encode: %v", err)
	}
}

// handleGetRoom: GET /api/rooms/{id}. Returns the current content as
// text/plain, or 404.
func (s *Server) handleGetRoom(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	room, err := s.store.Get(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Write(room.Content)
}
