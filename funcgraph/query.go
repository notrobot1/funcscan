package funcgraph

import "fmt"

// Paths returns all simple (cycle-free) paths from function from to function
// to, with at most maxDepth edges. Returns an empty slice when no path exists.
// maxDepth ≤ 0 defaults to 10.
func (l *LatticeDB) Paths(from, to string, maxDepth int) ([][]FuncNode, error) {
	if maxDepth <= 0 {
		maxDepth = 10
	}
	startFn, err := l.Function(from)
	if err != nil {
		return nil, fmt.Errorf("funcgraph: paths: source %w", err)
	}
	if _, err := l.Function(to); err != nil {
		return nil, fmt.Errorf("funcgraph: paths: target %w", err)
	}

	var results [][]FuncNode
	visited := make(map[string]bool)

	var dfs func(id string, path []FuncNode) error
	dfs = func(id string, path []FuncNode) error {
		if id == to {
			cp := make([]FuncNode, len(path))
			copy(cp, path)
			results = append(results, cp)
			return nil
		}
		if len(path) >= maxDepth+1 {
			return nil
		}
		visited[id] = true
		defer func() { delete(visited, id) }()

		callees, err := l.Callees(id)
		if err != nil {
			return err
		}
		for _, callee := range callees {
			if !visited[callee.ID] {
				if err := dfs(callee.ID, append(path, callee)); err != nil {
					return err
				}
			}
		}
		return nil
	}

	if err := dfs(from, []FuncNode{startFn}); err != nil {
		return nil, err
	}
	return results, nil
}

// Subgraph returns the subgraph centered on funcID, traversing up to depth
// hops in the given Direction. Nodes and edges that fall within the
// neighbourhood are included.
func (l *LatticeDB) Subgraph(funcID string, depth int, dir Direction) (*Subgraph, error) {
	if depth <= 0 {
		depth = 3
	}
	nodeSet := make(map[string]FuncNode)
	edgeSet := make(map[string]Edge)

	var walk func(id string, rem int) error
	walk = func(id string, rem int) error {
		if _, ok := nodeSet[id]; ok {
			return nil
		}
		fn, err := l.Function(id)
		if err != nil {
			return nil
		}
		nodeSet[id] = fn
		if rem == 0 {
			return nil
		}
		if dir == DirectionCallers || dir == DirectionBoth {
			edges, _ := l.InEdges(id)
			for _, e := range edges {
				edgeSet[e.ID] = e
				walk(e.From, rem-1) //nolint:errcheck
			}
		}
		if dir == DirectionCallees || dir == DirectionBoth {
			edges, _ := l.OutEdges(id)
			for _, e := range edges {
				edgeSet[e.ID] = e
				walk(e.To, rem-1) //nolint:errcheck
			}
		}
		return nil
	}

	if err := walk(funcID, depth); err != nil {
		return nil, err
	}

	sg := &Subgraph{
		Nodes: make([]FuncNode, 0, len(nodeSet)),
		Edges: make([]Edge, 0, len(edgeSet)),
	}
	for _, fn := range nodeSet {
		sg.Nodes = append(sg.Nodes, fn)
	}
	for _, e := range edgeSet {
		sg.Edges = append(sg.Edges, e)
	}
	return sg, nil
}
