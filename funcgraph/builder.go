package funcgraph

import (
	"fmt"
	"path/filepath"
	"strconv"

	funcscan "github.com/notrobot1/funcscan/relationships"
)

// ScanAndStore scans the source tree at root, discovers all named function
// definitions, infers call-dependency edges via the relationships package,
// and stores the resulting graph in the database.
//
// It performs two passes:
//  1. Collect every function definition → store as FuncNode.
//  2. For each unique function name, find all call sites and add edges.
func (l *LatticeDB) ScanAndStore(root string, opts *ScanOptions) error {
	fsOpts := &funcscan.Options{CodeOnly: true}
	if opts != nil {
		fsOpts.SkipDirs = opts.SkipDirs
		fsOpts.MaxFileSize = opts.MaxFileSize
	}

	// Phase 1: discover all function definitions.
	defs, err := funcscan.Definitions(root, fsOpts)
	if err != nil {
		return fmt.Errorf("funcgraph: scan definitions: %w", err)
	}

	// Store nodes and build a name → []FuncNode lookup table.
	byName := make(map[string][]FuncNode, len(defs))
	for _, d := range defs {
		fn := FuncNode{
			ID:        nodeID(d.File, d.StartLine),
			Name:      d.Name,
			File:      d.File,
			StartLine: d.StartLine,
			EndLine:   d.EndLine,
			Language:  d.Language,
		}
		if err := l.AddFunction(fn); err != nil {
			return fmt.Errorf("funcgraph: store %s: %w", fn.ID, err)
		}
		byName[fn.Name] = append(byName[fn.Name], fn)
	}

	// Phase 2: for each unique name find call sites → build edges.
	seen := make(map[string]bool, len(byName))
	for _, d := range defs {
		if seen[d.Name] {
			continue
		}
		seen[d.Name] = true

		refs, err := funcscan.FindByName(root, d.Name)
		if err != nil {
			continue
		}
		for _, ref := range refs {
			if ref.Kind != funcscan.KindCall && ref.Kind != funcscan.KindArgument {
				continue
			}
			if ref.Enclosing == nil {
				continue
			}

			callerNode := bestMatch(byName[ref.Enclosing.Name], ref.File,
				ref.Enclosing.StartLine, ref.Enclosing.EndLine)
			if callerNode == nil {
				continue
			}
			calleeNodes := byName[d.Name]
			if len(calleeNodes) == 0 {
				continue
			}
			calleeNode := bestMatch(calleeNodes, ref.File, 0, 0)
			if calleeNode == nil {
				calleeNode = &calleeNodes[0]
			}

			kind := EdgeCall
			if ref.Kind == funcscan.KindArgument {
				kind = EdgeArgument
			}
			if err := l.AddEdge(Edge{
				From: callerNode.ID,
				To:   calleeNode.ID,
				Kind: kind,
				File: ref.File,
				Line: ref.Line,
			}); err != nil {
				return fmt.Errorf("funcgraph: add edge: %w", err)
			}
		}
	}
	return nil
}

// nodeID returns a stable unique ID for a function definition.
func nodeID(file string, startLine int) string {
	return filepath.ToSlash(file) + ":" + strconv.Itoa(startLine)
}

// bestMatch finds the FuncNode from candidates that best matches file and
// line range. Priority: same file + line overlap > same file > first match.
func bestMatch(candidates []FuncNode, file string, startLine, endLine int) *FuncNode {
	if len(candidates) == 0 {
		return nil
	}
	file = filepath.ToSlash(file)
	if startLine > 0 {
		for i := range candidates {
			c := &candidates[i]
			if filepath.ToSlash(c.File) == file &&
				c.StartLine <= startLine && (endLine == 0 || c.EndLine >= endLine) {
				return c
			}
		}
	}
	for i := range candidates {
		if filepath.ToSlash(candidates[i].File) == file {
			return &candidates[i]
		}
	}
	return &candidates[0]
}
