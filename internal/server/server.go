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

type Config struct {
	BaseURL            string // public address, e.g. https://axshare.dev
	GitHubClientID     string
	GitHubClientSecret string
	GoogleClientID     string
	GoogleClientSecret string
}

// GitHubEnabled reports whether GitHub sign-in has everything it needs.
func (c Config) GitHubEnabled() bool {
	return c.BaseURL != "" && c.GitHubClientID != "" && c.GitHubClientSecret != ""
}

// GoogleEnabled reports whether Google sign-in has everything it needs.
func (c Config) GoogleEnabled() bool {
	return c.BaseURL != "" && c.GoogleClientID != "" && c.GoogleClientSecret != ""
}

type Server struct {
	mux       *http.ServeMux
	store     store.Store
	hub       *hub.Hub
	static    fs.FS
	cfg       Config
	providers []*provider
}

func New(addr string, st store.Store, h *hub.Hub, cfg Config) *http.Server {
	s := &Server{
		mux:       http.NewServeMux(),
		store:     st,
		hub:       h,
		cfg:       cfg,
		providers: newProviders(cfg),
	}
	s.routes()
	rl := newRateLimiter(5, 20)

	return &http.Server{
		Addr:    addr,
		Handler: logRequests(rl.middleware(s.withUser(s.mux))),
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
	s.mux.HandleFunc("PATCH /api/rooms/{id}", s.handleSetExt)
	s.mux.HandleFunc("GET /api/rooms/{id}", s.handleGetRoom)
	s.mux.HandleFunc("GET /api/me", s.handleMe)
	s.mux.HandleFunc("GET /ws/{id}", s.handleWS)

	s.mux.HandleFunc("GET /auth/{provider}/login", s.handleLogin)
	s.mux.HandleFunc("GET /auth/{provider}/callback", s.handleCallback)
	s.mux.HandleFunc("POST /auth/logout", s.handleLogout)
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
