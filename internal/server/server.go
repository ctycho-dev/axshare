package server

import (
	"embed"
	"encoding/json"
	"github.com/ctycho-dev/axshare/internal/store"
	"io/fs"
	"log"
	"net/http"
	"time"
)

var staticFS embed.FS

type Server struct {
	mux   *http.ServeMux
	store store.Store
}

func New(addr string, st store.Store) *http.Server {
	s := &Server{mux: http.NewServeMux(), store: st}
	s.routes()

	return &http.Server{
		Addr:    addr,
		Handler: s.mux,
		// Without this a client that opens a connection and never sends
		// headers holds a goroutine forever.
		ReadHeaderTimeout: 5 * time.Second,
	}
}

func (s *Server) routes() {
	// staticFS is rooted at the package directory, so files are at
	// "static/index.html". fs.Sub re-roots it so "/" maps to "index.html".
	static, err := fs.Sub(staticFS, "static")
	if err != nil {
		// embed is resolved at compile time; if Sub fails the program is
		// wrong, not the environment. Panicking during startup is fine.
		panic(err)
	}

	// Go 1.22+ patterns: "METHOD /path". Most specific pattern wins, so
	// /healthz is served by handleHealth and everything else by the
	// file server.
	s.mux.Handle("GET /", http.FileServerFS(static))
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
	s.mux.HandleFunc("POST /api/pastes", s.handleCreatePaste)
	s.mux.HandleFunc("GET /api/pastes/{id}", s.handleGetPaste)
}

type healthResponse struct {
	Status string `json:"status"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	err := json.NewEncoder(w).Encode(healthResponse{Status: "ok"})
	if err != nil {
		log.Printf("health: encode: %v", err)
	}
}
