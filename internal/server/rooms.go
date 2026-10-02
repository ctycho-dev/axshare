package server

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
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
	w.Header().Set("X-Room-Ext", room.Ext)
	w.Header().Set("X-Room-Expires", strconv.FormatInt(room.ExpiresAt.Unix(), 10))
	w.Write(room.Content)
}

// handleSetExt: PATCH /api/rooms/{id}. Body is {"ext": "go"}. Responds 204,
// 400 for an invalid ext, or 404 for an unknown room.
func (s *Server) handleSetExt(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var body struct {
		Ext string `json:"ext"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if !store.ValidExt(body.Ext) {
		http.Error(w, "invalid ext", http.StatusBadRequest)
		return
	}

	err := s.store.SetExt(r.Context(), id, body.Ext)
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
