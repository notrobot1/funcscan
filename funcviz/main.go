// Command funcviz scans a source tree, builds a function call-dependency graph,
// persists it in an embedded LatticeDB file, and serves an interactive web UI.
//
// Usage:
//
//	funcviz [flags] [path]
//
// Flags and the path argument may appear in any order.
//
//	-db   string   path to the graph database file
//	                 (default: <path>/.funcgraph.db)
//	-port int      HTTP port for the web UI (default: 8080)
//	-host string   host to bind to          (default: localhost)
//	-skip string   comma-separated extra dirs to skip
//	-rebuild       drop and recreate the database even if it already exists
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/notrobot1/funcscan/funcgraph"
	"github.com/notrobot1/funcscan/funcweb"
)

func main() {
	// Parse flags manually so they can appear in any order relative to the
	// positional path argument (the standard flag package stops at the first
	// non-flag argument, which breaks  ./funcviz -port 9090 ./myproject).
	cfg := struct {
		db      string
		port    int
		host    string
		skip    string
		rebuild bool
	}{
		port: 8080,
		host: "localhost",
	}

	var positional []string
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "-") {
			positional = append(positional, a)
			continue
		}
		name := strings.TrimLeft(a, "-")
		// Support both  -flag value  and  -flag=value
		var value string
		if idx := strings.IndexByte(name, '='); idx >= 0 {
			value = name[idx+1:]
			name = name[:idx]
		} else if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
			// peek at next token only for flags that take a value
			switch name {
			case "db", "port", "host", "skip":
				i++
				value = args[i]
			}
		}
		switch name {
		case "db":
			cfg.db = value
		case "port":
			v, err := strconv.Atoi(value)
			if err != nil {
				fatal("invalid -port value %q: %v", value, err)
			}
			cfg.port = v
		case "host":
			cfg.host = value
		case "skip":
			cfg.skip = value
		case "rebuild":
			cfg.rebuild = true
		case "h", "help":
			printUsage()
			os.Exit(0)
		default:
			fatal("unknown flag -%s\nRun funcviz -help for usage.", name)
		}
	}

	root := "."
	if len(positional) > 0 {
		root = positional[0]
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		fatal("resolve root path: %v", err)
	}
	if _, err := os.Stat(absRoot); err != nil {
		fatal("root path does not exist: %v", err)
	}

	// Default db path is inside the scanned directory.
	dbPath := cfg.db
	if dbPath == "" {
		dbPath = filepath.Join(absRoot, ".funcgraph.db")
	}

	// ── Open or create database ──────────────────────────────────────────
	if cfg.rebuild {
		if err := os.Remove(dbPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			fatal("remove old database: %v", err)
		}
		log.Printf("removed old database, rebuilding…")
	}

	dbExists := fileExists(dbPath)

	log.Printf("database: %s", dbPath)
	db, err := funcgraph.Open(dbPath)
	if err != nil {
		fatal("open database: %v", err)
	}
	defer db.Close()

	// ── Scan source tree ─────────────────────────────────────────────────
	if !dbExists || cfg.rebuild {
		opts := &funcgraph.ScanOptions{}
		if cfg.skip != "" {
			opts.SkipDirs = strings.Split(cfg.skip, ",")
		}

		log.Printf("scanning %s …", absRoot)
		start := time.Now()
		if err := db.ScanAndStore(absRoot, opts); err != nil {
			fatal("scan: %v", err)
		}

		fns, _   := db.Functions()
		edges, _ := db.AllEdges()
		log.Printf("done in %s — %d functions, %d edges",
			time.Since(start).Round(time.Millisecond), len(fns), len(edges))
	} else {
		fns, _   := db.Functions()
		edges, _ := db.AllEdges()
		log.Printf("using existing database: %d functions, %d edges", len(fns), len(edges))
		log.Printf("(pass -rebuild to rescan)")
	}

	// ── Start web server ─────────────────────────────────────────────────
	srv := funcweb.New(funcweb.Config{
		Host:    cfg.host,
		Port:    cfg.port,
		DB:      db,
		RootDir: absRoot,
	})

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-stop
		log.Println("shutting down…")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(ctx) //nolint:errcheck
	}()

	log.Printf("listening on %s", srv.Addr())
	fmt.Printf("\n  → open %s in your browser\n\n", srv.Addr())

	if err := srv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fatal("server: %v", err)
	}
}

func printUsage() {
	fmt.Print(`funcviz — function call-graph visualiser

Usage:
  funcviz [flags] [path]

  path   directory to scan (default: current directory)

Flags:
  -db <file>     graph database file
                   default: <path>/.funcgraph.db
  -port <int>    HTTP port  (default: 8080)
  -host <str>    bind host  (default: localhost)
  -skip <dirs>   comma-separated extra dirs to skip during scan
  -rebuild       delete and rebuild the database from scratch

Examples:
  funcviz ./myproject
  funcviz -port 9090 ./myproject
  funcviz -rebuild -port 9090 ./myproject
  funcviz -db /tmp/graph.db ./myproject
`)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "funcviz: "+format+"\n", args...)
	os.Exit(1)
}
