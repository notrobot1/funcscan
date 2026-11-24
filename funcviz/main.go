// Command funcviz scans a source tree, builds a function call-dependency graph,
// persists it in an embedded LatticeDB file, and serves an interactive web UI.
//
//	funcviz [flags] [path]
//
// Flags:
//
//	-db   string   path to the graph database file (default: funcgraph.db)
//	-port int      HTTP port for the web UI          (default: 8080)
//	-host string   host to bind to                   (default: localhost)
//	-skip string   comma-separated dirs to skip       (adds to default list)
//	-rebuild       drop and recreate the database even if it already exists
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
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/notrobot1/funcscan/funcgraph"
	"github.com/notrobot1/funcscan/funcweb"
)

func main() {
	dbPath  := flag.String("db",      "funcgraph.db", "path to graph database file")
	port    := flag.Int("port",       8080,           "HTTP port")
	host    := flag.String("host",    "localhost",    "host to bind to")
	skip    := flag.String("skip",    "",             "extra dirs to skip (comma-separated)")
	rebuild := flag.Bool("rebuild",   false,          "rebuild the database from scratch")
	flag.Parse()

	root := "."
	if flag.NArg() > 0 {
		root = flag.Arg(0)
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		fatal("resolve root path: %v", err)
	}
	if _, err := os.Stat(absRoot); err != nil {
		fatal("root path does not exist: %v", err)
	}

	// ── Open or create database ──────────────────────────────────────────
	if *rebuild {
		if err := os.Remove(*dbPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			fatal("remove old database: %v", err)
		}
	}

	dbExists := fileExists(*dbPath)

	log.Printf("opening database: %s", *dbPath)
	db, err := funcgraph.Open(*dbPath)
	if err != nil {
		fatal("open database: %v", err)
	}
	defer db.Close()

	// ── Scan source tree (skip if db already populated and -rebuild not set) ──
	if !dbExists || *rebuild {
		opts := &funcgraph.ScanOptions{}
		if *skip != "" {
			opts.SkipDirs = strings.Split(*skip, ",")
		}

		log.Printf("scanning %s …", absRoot)
		start := time.Now()
		if err := db.ScanAndStore(absRoot, opts); err != nil {
			fatal("scan: %v", err)
		}

		fns, _ := db.Functions()
		edges, _ := db.AllEdges()
		log.Printf("done in %s — %d functions, %d edges",
			time.Since(start).Round(time.Millisecond), len(fns), len(edges))
	} else {
		fns, _   := db.Functions()
		edges, _ := db.AllEdges()
		log.Printf("using existing database: %d functions, %d edges (use -rebuild to rescan)", len(fns), len(edges))
	}

	// ── Start web server ─────────────────────────────────────────────────
	srv := funcweb.New(funcweb.Config{
		Host:    *host,
		Port:    *port,
		DB:      db,
		RootDir: absRoot,
	})

	// Graceful shutdown on SIGINT / SIGTERM.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-stop
		log.Println("shutting down…")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(ctx) //nolint:errcheck
	}()

	log.Printf("web UI: %s", srv.Addr())
	fmt.Printf("\n  → open %s in your browser\n\n", srv.Addr())

	if err := srv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fatal("server: %v", err)
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "funcviz: "+format+"\n", args...)
	os.Exit(1)
}
