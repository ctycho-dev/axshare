package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/ctycho-dev/axshare/internal/hub"
	"github.com/ctycho-dev/axshare/internal/server"
	"github.com/ctycho-dev/axshare/internal/store"
)

const version = "0.1.0"

func main() {
	addr := flag.String("addr", ":8070", "address to listen on")
	dbPath := flag.String("db", "axshare.db", "path to the SQLite file")
	noRateLimit := flag.Bool("no-ratelimit", false, "disable the per-IP rate limit (load testing only)")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("axshare", version)
		return
	}

	// Secrets come from the environment, not flags: flags are visible to
	// anyone who can list processes on the machine.
	cfg := server.Config{
		BaseURL:            strings.TrimRight(os.Getenv("BASE_URL"), "/"),
		GitHubClientID:     os.Getenv("GITHUB_CLIENT_ID"),
		GitHubClientSecret: os.Getenv("GITHUB_CLIENT_SECRET"),
		GoogleClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		DisableRateLimit:   *noRateLimit,
	}

	if err := run(*addr, *dbPath, cfg); err != nil {
		log.Println("axshare:", err)
		os.Exit(1)
	}
}

func run(addr, dbPath string, cfg server.Config) error {
	// ctx is cancelled on Ctrl+C or SIGTERM (what Docker/systemd send).
	// Everything long-running below takes it, so one signal unwinds all.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := store.OpenSQLite(dbPath)
	if err != nil {
		return err
	}
	defer st.Close()

	h := hub.New(st)
	srv := server.New(addr, st, h, cfg)
	log.Printf("sign-in: github=%v google=%v", cfg.GitHubEnabled(), cfg.GoogleEnabled())
	if cfg.DisableRateLimit {
		log.Println("WARNING: rate limiting is disabled; do not run production like this")
	}

	// Expiry runs in the background until ctx is cancelled.
	go store.RunExpiry(ctx, st, time.Minute)

	// ListenAndServe blocks, so it runs in its own goroutine and reports
	// through a channel. Buffered (size 1) so the goroutine can exit even
	// if nobody is listening any more.
	errCh := make(chan error, 1)
	go func() {
		log.Printf("axshare %s listening on %s (db: %s)", version, addr, dbPath)
		errCh <- srv.ListenAndServe()
	}()

	// Wait for whichever happens first: the server dies, or a signal.
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		log.Println("shutting down")
	}

	// Graceful stop: refuse new connections, finish in-flight requests,
	// but give up after 10s. Shutdown does not touch hijacked WebSocket
	// connections, so the hub closes those itself.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	h.Close()
	if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("shutdown: %w", err)
	}
	return nil
}
