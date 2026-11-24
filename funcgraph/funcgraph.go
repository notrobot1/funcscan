// Package funcgraph builds and queries a function call-dependency graph,
// persisting it in an embedded LatticeDB (bbolt-backed) database.
//
// Typical workflow:
//
//	db, _ := funcgraph.Open("graph.db")
//	defer db.Close()
//	db.ScanAndStore("/path/to/project", nil)
//	callees, _ := db.Callees("main.go:5")
package funcgraph

// FuncNode represents a function definition stored in the graph.
type FuncNode struct {
	ID        string `json:"id"`         // unique key: "file:startLine"
	Name      string `json:"name"`
	File      string `json:"file"`       // path relative to scanned root
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Language  string `json:"language"`
}

// EdgeKind classifies a dependency edge.
type EdgeKind string

const (
	EdgeCall      EdgeKind = "call"
	EdgeArgument  EdgeKind = "argument"
	EdgeReference EdgeKind = "reference"
)

// Edge is a directed dependency from one function to another.
type Edge struct {
	ID   string   `json:"id"`
	From string   `json:"from"` // FuncNode.ID of the caller
	To   string   `json:"to"`   // FuncNode.ID of the callee
	Kind EdgeKind `json:"kind"`
	File string   `json:"file"` // file where the call site appears
	Line int      `json:"line"` // line of the call site
}

// ScanOptions controls how ScanAndStore explores a source tree.
// A nil value uses safe defaults (same as relationships.DefaultSkipDirs, 2 MiB limit).
type ScanOptions struct {
	SkipDirs    []string
	MaxFileSize int64
	// Progress is called periodically during scanning.
	// phase is "definitions" or "edges", done and total are item counts.
	// May be nil.
	Progress func(phase string, done, total int)
}

// Direction controls traversal direction in graph queries.
type Direction int

const (
	DirectionCallers Direction = iota // upstream: who calls this function
	DirectionCallees                  // downstream: what this function calls
	DirectionBoth                     // both directions
)

// Subgraph is the result of a neighborhood query.
type Subgraph struct {
	Nodes []FuncNode
	Edges []Edge
}
