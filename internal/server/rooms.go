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

const maxRoomBytes = 1 << 20 // 1 MiB

// handleCreateRoom: POST /api/rooms. Body is the initial content (may be
// empty). Responds 201 with {"id": "..."}. A signed-in user becomes the
// room's owner, which also gives it the longer lifetime.
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

	var ownerID int64 // stays 0 for an anonymous request
	if u, ok := userFrom(r.Context()); ok {
		ownerID = u.ID
	}

	now := time.Now()
	room := store.Room{
		ID:        id,
		Content:   content,
		OwnerID:   ownerID,
		CreatedAt: now,
		ExpiresAt: now.Add(store.TTL(ownerID)),
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
// text/plain, with the file type, expiry and lifetime in headers, or 404.
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

	ttl := int64(store.TTL(room.OwnerID).Seconds())

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Room-Ext", room.Ext)
	w.Header().Set("X-Room-Expires", strconv.FormatInt(room.ExpiresAt.Unix(), 10))
	w.Header().Set("X-Room-TTL", strconv.FormatInt(ttl, 10))
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

type roomInfoResponse struct {
	ID      string `json:"id"`
	Ext     string `json:"ext"`
	Bytes   int    `json:"bytes"`
	Expires int64  `json:"expires"` // Unix seconds
}

// handleMyRooms: GET /api/me/rooms. Lists the signed-in user's rooms, most
// recently edited first. 401 when anonymous.
func (s *Server) handleMyRooms(w http.ResponseWriter, r *http.Request) {
	u, ok := userFrom(r.Context())
	if !ok {
		http.Error(w, "not signed in", http.StatusUnauthorized)
		return
	}

	rooms, err := s.store.RoomsByOwner(r.Context(), u.ID)
	if err != nil {
		log.Printf("my rooms: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// make with length 0, not a nil slice: a nil slice encodes as JSON
	// null, and the page expects a list even when it is empty.
	out := make([]roomInfoResponse, 0, len(rooms))
	for _, ri := range rooms {
		out = append(out, roomInfoResponse{
			ID:      ri.ID,
			Ext:     ri.Ext,
			Bytes:   ri.Bytes,
			Expires: ri.ExpiresAt.Unix(),
		})
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(out); err != nil {
		log.Printf("my rooms: encode: %v", err)
	}
}
