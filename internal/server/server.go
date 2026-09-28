package server

import (
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"time"

	"github.com/ctycho-dev/axshare/internal/hub"
	"github.com/ctycho-dev/axshare/internal/store"
)

// The next line is a compiler directive, not a comment. It must sit
// directly above the var, with no blank line between them.
//
//go:embed static
var staticFS embed.FS

type Server struct {
	mux   *http.ServeMux
	store store.Store
	hub   *hub.Hub
}

func New(addr string, st store.Store, h *hub.Hub) *http.Server {
	s := &Server{mux: http.NewServeMux(), store: st, hub: h}
	s.routes()

	return &http.Server{
		Addr:    addr,
		Handler: logRequests(s.mux),
		// Without this a client that opens a connection and never sends
		// headers holds a goroutine forever.
		ReadHeaderTimeout: 5 * time.Second,
	}
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}

func (s *Server) routes() {
	// staticFS is the package-level variable above. Do not redeclare it
	// here: a local with the same name would hide it and be empty.
	static, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err)
	}

	s.mux.Handle("GET /", http.FileServerFS(static))
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
	s.mux.HandleFunc("POST /api/pastes", s.handleCreatePaste)
	s.mux.HandleFunc("GET /api/pastes/{id}", s.handleGetPaste)
	s.mux.HandleFunc("GET /ws/{id}", s.handleWS)
}

type healthResponse struct {
	Status  string `json:"status"`
	Clients int    `json:"clients"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	err := json.NewEncoder(w).Encode(healthResponse{
		Status:  "ok",
		Clients: s.hub.ActiveClients(),
	})
	if err != nil {
		log.Printf("health: encode: %v", err)
	}
}
