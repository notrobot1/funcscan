// Package funcweb serves a web UI that visualises the function dependency
// graph stored in a funcgraph.LatticeDB instance.
package funcweb

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/notrobot1/funcscan/funcgraph"
)

// Config holds all options for the web server.
type Config struct {
	// Host to bind to. Defaults to "localhost".
	Host string
	// Port to listen on. Defaults to 8080.
	Port int
	// DB is the graph database to visualise (required).
	DB *funcgraph.LatticeDB
	// RootDir is the source-code root used when serving file contents.
	// When empty the server still serves graph data but file preview is disabled.
	RootDir string
}

// Server is the HTTP server for the graph web UI.
type Server struct {
	cfg Config
	srv *http.Server
}

// New creates a new Server with the given configuration.
// Call Start to begin accepting connections.
func New(cfg Config) *Server {
	if cfg.Host == "" {
		cfg.Host = "localhost"
	}
	if cfg.Port == 0 {
		cfg.Port = 8080
	}

	mux := http.NewServeMux()
	s := &Server{cfg: cfg}
	s.registerRoutes(mux)

	s.srv = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	return s
}

// Addr returns the base URL of the server (e.g. "http://localhost:8080").
func (s *Server) Addr() string {
	return fmt.Sprintf("http://%s:%d", s.cfg.Host, s.cfg.Port)
}

// Start starts the HTTP server and blocks until it is stopped.
// Returns http.ErrServerClosed when Shutdown is called.
func (s *Server) Start() error {
	return s.srv.ListenAndServe()
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}
