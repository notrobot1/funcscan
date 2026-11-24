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
// ScanAndStore scans the source tree at root, discovers all named function
// definitions, infers call-dependency edges via the relationships package,
// and stores the resulting graph in the database.
//
// The scan is resumable: if a previous run was interrupted, the next call with
// the same root will continue from the last saved checkpoint rather than
// starting over. Pass opts.Resume = false (or use -rebuild in the CLI) to
// force a full re-scan.
func (l *LatticeDB) ScanAndStore(root string, opts *ScanOptions) error {
	fsOpts := &funcscan.Options{CodeOnly: true}
	var progress func(phase string, done, total int)
	if opts != nil {
		fsOpts.SkipDirs = opts.SkipDirs
		fsOpts.MaxFileSize = opts.MaxFileSize
		progress = opts.Progress
	}
	report := func(phase string, done, total int) {
		if progress != nil {
			progress(phase, done, total)
		}
	}

	// ── Check for a resumable interrupted scan ───────────────────────────
	state, err := l.ScanStateGet()
	if err != nil {
		return fmt.Errorf("funcgraph: read scan state: %w", err)
	}

	resuming := state.Status == scanStatusScanning &&
		state.Root == root &&
		state.Phase == "2"

	var byName map[string][]FuncNode

	if resuming {
		// Phase 1 already completed — rebuild byName from stored nodes.
		report("definitions", 0, 0)
		existing, err := l.Functions()
		if err != nil {
			return fmt.Errorf("funcgraph: load existing nodes: %w", err)
		}
		byName = make(map[string][]FuncNode, len(existing))
		for _, fn := range existing {
			byName[fn.Name] = append(byName[fn.Name], fn)
		}
		report("definitions", len(existing), len(existing))
	} else {
		// Fresh scan: clear any stale checkpoint data.
		if err := l.ScanStateClear(); err != nil {
			return fmt.Errorf("funcgraph: clear scan state: %w", err)
		}
		if err := l.scanStateSet(scanStatusScanning, "1", root); err != nil {
			return err
		}

		// Phase 1: discover all function definitions.
		report("definitions", 0, 0)
		defs, err := funcscan.Definitions(root, fsOpts)
		if err != nil {
			return fmt.Errorf("funcgraph: scan definitions: %w", err)
		}

		byName = make(map[string][]FuncNode, len(defs))
		for i, d := range defs {
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
			if i%50 == 0 {
				report("definitions", i+1, len(defs))
			}
		}
		report("definitions", len(defs), len(defs))

		if err := l.scanStateSet(scanStatusScanning, "2", root); err != nil {
			return err
		}
	}

	// ── Phase 2: for each unique name find call sites → build edges ──────
	// Load the set of names already processed in a previous (interrupted) run.
	alreadyDone, err := l.scanIsDone()
	if err != nil {
		return fmt.Errorf("funcgraph: load checkpoint: %w", err)
	}

	uniqueNames := make([]string, 0, len(byName))
	for name := range byName {
		uniqueNames = append(uniqueNames, name)
	}
	total := len(uniqueNames)
	done  := len(alreadyDone)

	for _, d := range uniqueNames {
		if _, skip := alreadyDone[d]; skip {
			continue
		}

		done++
		report("edges", done, total)

		refs, err := funcscan.FindByName(root, d)
		if err != nil {
			// Mark as processed even on error so we don't retry endlessly.
			l.scanMarkDone(d) //nolint:errcheck
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
			calleeNodes := byName[d]
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

		// Checkpoint: mark this name as fully processed.
		if err := l.scanMarkDone(d); err != nil {
			return fmt.Errorf("funcgraph: checkpoint: %w", err)
		}
	}

	// Mark scan complete so the next run won't re-scan.
	return l.scanStateSet(scanStatusDone, "2", root)
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
