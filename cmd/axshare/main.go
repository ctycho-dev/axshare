package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"github.com/ctycho-dev/axshare/internal/store"
	"github.com/ctycho-dev/axshare/internal/server"
)

const version = "0.1.0"

func main() {
	addr := flag.String("addr", ":8070", "address to listen on")
	dbPath := flag.String("db", "axshare.db", "path to the SQLite file (used from stage 4)")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("axshare", version)
		return
	}
	// main() only parses flags and reports errors. All real work lives in
	// run() so it can return an error like any other function and so
	// deferred cleanup runs — os.Exit skips defers.
	if err := run(*addr, *dbPath); err != nil {
		log.Println("axshare:", err)
		os.Exit(1)
	}
}

func run(addr, dbPath string) error {
	st, err := store.OpenSQLite(dbPath)
	if err != nil {
		return err
	}
	defer st.Close()

	srv := server.New(addr, st)

	log.Printf("axshare listening on %s (db: %s)", addr, dbPath)
	return srv.ListenAndServe()
}
