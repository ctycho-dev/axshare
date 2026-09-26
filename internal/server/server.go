// Package server wires HTTP routes to handlers. It knows nothing about
// storage yet; that arrives in stage 3 through a store.Store interface.
package server

import (
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"time"
)

// The static/ directory is compiled into the binary at build time.
// The path is relative to this file and resolved by the compiler,
// so a typo here is a compile error, not a runtime surprise.
//
//go:embed static
var staticFS embed.FS

// Server holds the router and, later, the store and the hub.
// Handlers are methods on it so they can reach those dependencies
// without globals.
type Server struct {
	mux *http.ServeMux
}

// New builds the router and returns a ready-to-run http.Server.
// Returning *http.Server (a concrete type) rather than an interface
// is the "accept interfaces, return structs" guideline in practice.
func New(addr string) *http.Server {
	s := &Server{mux: http.NewServeMux()}
	s.routes()

	return &http.Server{
		Addr:    addr,
		Handler: s.mux,
		// Without this a client that opens a connection and never sends
		// headers holds a goroutine forever.
		ReadHeaderTimeout: 5 * time.Second,
	}
}

// routes is the one place that lists every URL the server answers.
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
