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
//go:embed all:dist
var staticFS embed.FS

type Server struct {
	mux    *http.ServeMux
	store  store.Store
	hub    *hub.Hub
	static fs.FS
}

func New(addr string, st store.Store, h *hub.Hub) *http.Server {
	s := &Server{mux: http.NewServeMux(), store: st, hub: h}
	s.routes()
	rl := newRateLimiter(5, 20)

	return &http.Server{
		Addr:    addr,
		Handler: logRequests(rl.middleware(s.mux)),
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
	static, err := fs.Sub(staticFS, "dist")
	if err != nil {
		panic(err)
	}
	s.static = static

	s.mux.Handle("GET /", http.FileServerFS(static))
	s.mux.HandleFunc("GET /room/{id}", s.handleRoomPage)
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
	s.mux.HandleFunc("POST /api/rooms", s.handleCreateRoom)
	s.mux.HandleFunc("GET /api/rooms/{id}", s.handleGetRoom)
	s.mux.HandleFunc("GET /ws/{id}", s.handleWS)
}

func (s *Server) handleRoomPage(w http.ResponseWriter, r *http.Request) {
	http.ServeFileFS(w, r, s.static, "room.html")
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
