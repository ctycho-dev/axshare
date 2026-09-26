package server

import (
	"encoding/json"
	"errors"
	"github.com/ctycho-dev/axshare/internal/store"
	"io"
	"log"
	"net/http"
	"time"
)

func (s *Server) handleCreatePaste(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
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
	p := store.Paste{
		ID:        id,
		Content:   content,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	if err := s.store.Put(r.Context(), p); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(struct {
		ID string `json:"id"`
	}{id}); err != nil {
		log.Printf("create paste: encode: %v", err)
	}
}

func (s *Server) handleGetPaste(w http.ResponseWriter, r *http.Request) {
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

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Write(p.Content)
}
