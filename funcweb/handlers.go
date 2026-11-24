package funcweb

import (
	"encoding/json"
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
}

// handleGraph returns all nodes and edges as a single JSON object.
// For very large graphs clients should use /api/subgraph for incremental loading.
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

	type resp struct {
		Nodes []funcgraph.FuncNode `json:"nodes"`
		Edges []funcgraph.Edge     `json:"edges"`
	}
	writeJSON(w, resp{Nodes: nodes, Edges: edges})
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
