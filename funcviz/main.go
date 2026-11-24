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

// progress renders a single-line progress bar that overwrites itself in place.
// Call with done==total to print a final "done" line with a newline.
func progress(phase string, done, total int) {
	const barWidth = 30
	var bar string
	if total > 0 {
		filled := barWidth * done / total
		if filled > barWidth {
			filled = barWidth
		}
		bar = "[" + strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled) + "]"
	} else {
		bar = "[" + strings.Repeat("░", barWidth) + "]"
	}

	var pct string
	if total > 0 {
		pct = fmt.Sprintf("%3d%%", 100*done/total)
	} else {
		pct = "  ?%"
	}

	line := fmt.Sprintf("  %-12s %s %s  %d/%d", phase, bar, pct, done, total)
	if done == total && total > 0 {
		fmt.Printf("\r%s\n", line)
	} else {
		fmt.Printf("\r%s", line)
	}
}

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
		log.Printf("removed old database, rebuilding from scratch")
	}

	log.Printf("database: %s", dbPath)
	db, err := funcgraph.Open(dbPath)
	if err != nil {
		fatal("open database: %v", err)
	}
	defer db.Close()

	// ── Decide whether to scan ────────────────────────────────────────────
	state, err := db.ScanStateGet()
	if err != nil {
		fatal("read scan state: %v", err)
	}

	needScan := cfg.rebuild || state.Status != "done" || state.Root != absRoot

	if !needScan {
		fns, _   := db.Functions()
		edges, _ := db.AllEdges()
		log.Printf("graph ready: %d functions, %d edges (pass -rebuild to rescan)", len(fns), len(edges))
	} else {
		opts := &funcgraph.ScanOptions{
			Progress: progress,
		}
		if cfg.skip != "" {
			opts.SkipDirs = strings.Split(cfg.skip, ",")
		}

		switch {
		case cfg.rebuild:
			log.Printf("scanning %s (full rebuild)…", absRoot)
		case state.Status == "scanning" && state.Root == absRoot:
			fns, _  := db.Functions()
			done, _ := db.ScanStateGet()
			log.Printf("resuming interrupted scan of %s (%d functions already stored, phase %s)…",
				absRoot, len(fns), done.Phase)
		default:
			log.Printf("scanning %s …", absRoot)
		}

		start := time.Now()
		if err := db.ScanAndStore(absRoot, opts); err != nil {
			fmt.Println() // end progress line before error
			fatal("scan: %v", err)
		}

		fns, _   := db.Functions()
		edges, _ := db.AllEdges()
		log.Printf("done in %s — %d functions, %d edges",
			time.Since(start).Round(time.Millisecond), len(fns), len(edges))

		log.Printf("computing layout…")
		layoutStart := time.Now()
		if err := db.ComputeLayout(&funcgraph.LayoutOptions{
			Progress: func(iter, total int) { progress("layout", iter, total) },
		}); err != nil {
			fmt.Println()
			log.Printf("warning: layout failed: %v", err)
		} else {
			fmt.Println()
			log.Printf("layout done in %s", time.Since(layoutStart).Round(time.Millisecond))
		}
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
