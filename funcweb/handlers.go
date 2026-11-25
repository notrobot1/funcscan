package funcweb

import (
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/notrobot1/funcscan/funcgraph"
)

func (s *Server) registerRoutes(mux *http.ServeMux) {
	// Static assets — strip the "static/" prefix from embed paths.
	sub, _ := fs.Sub(staticFiles, "static")
	mux.Handle("/", http.FileServer(http.FS(sub)))

	// Graph data
	mux.HandleFunc("/api/graph", s.handleGraph)
	mux.HandleFunc("/api/subgraph/", s.handleSubgraph)
	mux.HandleFunc("/api/node/", s.handleNode)
	mux.HandleFunc("/api/file", s.handleFile)

	// Saved tabs (stored in DB)
	mux.HandleFunc("/api/saved", s.handleSaved)

	// Lazy function analysis (entry-point detection, etc.)
	mux.HandleFunc("/api/analyze/", s.handleAnalyze)

	// Terminal (SSE-streamed shell execution in RootDir)
	mux.HandleFunc("/api/terminal/run", s.handleTerminalRun)
}

// handleGraph returns all nodes, edges, and pre-computed layout positions.
func (s *Server) handleGraph(w http.ResponseWriter, r *http.Request) {
	nodes, err := s.cfg.DB.Functions()
	if err != nil {
		httpErr(w, err, http.StatusInternalServerError)
		return
	}
	edges, err := s.cfg.DB.AllEdges()
	if err != nil {
		httpErr(w, err, http.StatusInternalServerError)
		return
	}
	positions, err := s.cfg.DB.GetLayout()
	if err != nil {
		httpErr(w, err, http.StatusInternalServerError)
		return
	}

	type resp struct {
		Nodes     []funcgraph.FuncNode              `json:"nodes"`
		Edges     []funcgraph.Edge                   `json:"edges"`
		Positions map[string]funcgraph.Position      `json:"positions,omitempty"`
	}
	writeJSON(w, resp{Nodes: nodes, Edges: edges, Positions: positions})
}

// handleSubgraph returns the neighbourhood of a node.
// Path: /api/subgraph/{nodeID}
// Query params: depth (default 3), direction (callers|callees|both, default both)
func (s *Server) handleSubgraph(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/subgraph/")
	if id == "" {
		http.Error(w, "missing node id", http.StatusBadRequest)
		return
	}

	depth := 3
	if d := r.URL.Query().Get("depth"); d != "" {
		if v, err := strconv.Atoi(d); err == nil && v > 0 {
			depth = v
		}
	}

	dir := funcgraph.DirectionBoth
	switch r.URL.Query().Get("direction") {
	case "callers":
		dir = funcgraph.DirectionCallers
	case "callees":
		dir = funcgraph.DirectionCallees
	}

	sg, err := s.cfg.DB.Subgraph(id, depth, dir)
	if err != nil {
		httpErr(w, err, http.StatusInternalServerError)
		return
	}
	writeJSON(w, sg)
}

// handleNode returns a single function node.
// Path: /api/node/{nodeID}
func (s *Server) handleNode(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/node/")
	if id == "" {
		http.Error(w, "missing node id", http.StatusBadRequest)
		return
	}
	fn, err := s.cfg.DB.Function(id)
	if err != nil {
		httpErr(w, err, http.StatusNotFound)
		return
	}
	writeJSON(w, fn)
}

// handleFile returns the raw content of a source file together with language hint.
// Query params: path (relative to RootDir)
func (s *Server) handleFile(w http.ResponseWriter, r *http.Request) {
	if s.cfg.RootDir == "" {
		http.Error(w, "file preview not configured", http.StatusServiceUnavailable)
		return
	}
	rel := r.URL.Query().Get("path")
	if rel == "" {
		http.Error(w, "missing path", http.StatusBadRequest)
		return
	}
	// Prevent path traversal.
	clean := filepath.Clean(rel)
	if strings.HasPrefix(clean, "..") {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	full := filepath.Join(s.cfg.RootDir, clean)
	content, err := os.ReadFile(full)
	if err != nil {
		httpErr(w, err, http.StatusNotFound)
		return
	}

	type resp struct {
		Path     string `json:"path"`
		Language string `json:"language"`
		Content  string `json:"content"`
	}
	writeJSON(w, resp{
		Path:     rel,
		Language: extToLanguage(filepath.Ext(full)),
		Content:  string(content),
	})
}

// --- helpers ----------------------------------------------------------------

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "")
	enc.Encode(v) //nolint:errcheck
}

func httpErr(w http.ResponseWriter, err error, code int) {
	http.Error(w, err.Error(), code)
}

func extToLanguage(ext string) string {
	m := map[string]string{
		".go":   "go",
		".py":   "python",
		".js":   "javascript",
		".ts":   "typescript",
		".tsx":  "typescript",
		".rs":   "rust",
		".rb":   "ruby",
		".java": "java",
		".cpp":  "cpp",
		".cc":   "cpp",
		".cxx":  "cpp",
		".c":    "c",
		".cs":   "csharp",
		".kt":   "kotlin",
		".scala": "scala",
		".lua":  "lua",
		".php":  "php",
		".sh":   "bash",
	}
	if lang, ok := m[strings.ToLower(ext)]; ok {
		return lang
	}
	return "plaintext"
}

// handleSaved handles GET (read) and PUT (write) for saved tabs.
func (s *Server) handleSaved(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		data, err := s.cfg.DB.GetConfig("saved_tabs")
		if err != nil {
			httpErr(w, err, http.StatusInternalServerError)
			return
		}
		if len(data) == 0 {
			data = []byte(`{"tabs":[]}`)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)

	case http.MethodPut:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			httpErr(w, err, http.StatusBadRequest)
			return
		}
		if err := s.cfg.DB.SetConfig("saved_tabs", body); err != nil {
			httpErr(w, err, http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleAnalyze runs all analysis passes for a node and returns the result.
// Path: /api/analyze/{nodeID}
// Called lazily when the user first opens a function's source code.
func (s *Server) handleAnalyze(w http.ResponseWriter, r *http.Request) {
	if s.cfg.RootDir == "" {
		http.Error(w, "analysis not configured (no RootDir)", http.StatusServiceUnavailable)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/analyze/")
	if id == "" {
		http.Error(w, "missing node id", http.StatusBadRequest)
		return
	}
	fn, err := s.cfg.DB.Function(id)
	if err != nil {
		httpErr(w, err, http.StatusNotFound)
		return
	}
	result, err := s.AnalyzeFunction(fn)
	if err != nil {
		httpErr(w, err, http.StatusInternalServerError)
		return
	}
	writeJSON(w, result)
}
