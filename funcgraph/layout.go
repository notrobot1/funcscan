package funcgraph

import (
	"encoding/json"
	"math"
	"math/rand"

	bbolt "go.etcd.io/bbolt"
)

// ComputeLayout runs Fruchterman-Reingold force-directed layout on all nodes,
// resolves remaining overlaps, then stores the resulting positions in the DB.
//
// The algorithm guarantees:
//   - Connected nodes are attracted toward each other.
//   - All nodes repel each other so they spread out.
//   - A post-processing pass pushes overlapping nodes apart.
//   - Isolated nodes (no edges) are pinned in a compact grid to the right.
func (l *LatticeDB) ComputeLayout(opts *LayoutOptions) error {
	if opts == nil {
		opts = &LayoutOptions{}
	}
	nodeRadius := opts.NodeRadius
	if nodeRadius <= 0 {
		nodeRadius = 20
	}

	// ── Load graph data ──────────────────────────────────────────────────
	nodes, err := l.Functions()
	if err != nil {
		return err
	}
	edges, err := l.AllEdges()
	if err != nil {
		return err
	}
	if len(nodes) == 0 {
		return nil
	}

	n := len(nodes)

	// Auto-scale iterations: more nodes → fewer iterations per node is fine.
	iters := opts.Iterations
	if iters <= 0 {
		switch {
		case n <= 100:
			iters = 500
		case n <= 300:
			iters = 300
		case n <= 800:
			iters = 200
		default:
			iters = 100
		}
	}

	// ── Identify isolated nodes ──────────────────────────────────────────
	hasEdge := make(map[string]bool, len(edges)*2)
	for _, e := range edges {
		hasEdge[e.From] = true
		hasEdge[e.To] = true
	}

	idxByID := make(map[string]int, n)
	for i, nd := range nodes {
		idxByID[nd.ID] = i
	}

	// ── Initial positions ────────────────────────────────────────────────
	// Spread on a circle of radius r₀ so the force has room to work.
	type vec2 struct{ x, y float64 }
	pos := make([]vec2, n)
	r0 := math.Sqrt(float64(n)) * 150.0  // larger → more spread between nodes
	rng := rand.New(rand.NewSource(42))
	for i := range pos {
		angle := 2 * math.Pi * float64(i) / float64(n)
		jitter := (rng.Float64() - 0.5) * r0 * 0.1
		pos[i] = vec2{
			x: r0*math.Cos(angle) + jitter,
			y: r0*math.Sin(angle) + jitter,
		}
	}

	// ── Fruchterman-Reingold ─────────────────────────────────────────────
	// k = ideal spring length  (sqrt of area per node)
	area := r0 * r0 * 4
	k := math.Sqrt(area / float64(n))
	k2 := k * k

	// Initial temperature = fraction of the layout area width.
	// Cools geometrically to near-zero over `iters` steps.
	temp := r0 * 0.5
	cooling := math.Pow(0.001/temp, 1.0/float64(iters))

	disp := make([]vec2, n)

	for it := 0; it < iters; it++ {
		for i := range disp {
			disp[i] = vec2{}
		}

		// Repulsive force: every pair of nodes pushes each other apart.
		for i := 0; i < n; i++ {
			for j := i + 1; j < n; j++ {
				dx := pos[i].x - pos[j].x
				dy := pos[i].y - pos[j].y
				d := math.Sqrt(dx*dx + dy*dy)
				if d < 0.001 {
					d = 0.001
					dx = rng.Float64() - 0.5
					dy = rng.Float64() - 0.5
				}
				f := k2 / d
				nx, ny := dx/d*f, dy/d*f
				disp[i].x += nx
				disp[i].y += ny
				disp[j].x -= nx
				disp[j].y -= ny
			}
		}

		// Attractive force: connected nodes pull toward each other.
		for _, e := range edges {
			si, ok1 := idxByID[e.From]
			ti, ok2 := idxByID[e.To]
			if !ok1 || !ok2 || si == ti {
				continue
			}
			dx := pos[si].x - pos[ti].x
			dy := pos[si].y - pos[ti].y
			d := math.Sqrt(dx*dx + dy*dy)
			if d < 0.001 {
				continue
			}
			f := d * d / k
			nx, ny := dx/d*f, dy/d*f
			disp[si].x -= nx
			disp[si].y -= ny
			disp[ti].x += nx
			disp[ti].y += ny
		}

		// Apply displacements capped by temperature.
		for i := range pos {
			if !hasEdge[nodes[i].ID] {
				continue // isolated nodes are positioned separately below
			}
			d := math.Sqrt(disp[i].x*disp[i].x + disp[i].y*disp[i].y)
			if d < 0.001 {
				continue
			}
			factor := math.Min(d, temp) / d
			pos[i].x += disp[i].x * factor
			pos[i].y += disp[i].y * factor
		}

		temp *= cooling

		if opts.Progress != nil && it%10 == 0 {
			opts.Progress(it, iters)
		}
	}
	if opts.Progress != nil {
		opts.Progress(iters, iters)
	}

	// ── Overlap resolution ───────────────────────────────────────────────
	// Push overlapping connected-node pairs apart until no overlap remains
	// (or max passes exceeded).
	minDist := nodeRadius * 2
	for pass := 0; pass < 50; pass++ {
		moved := false
		for i := 0; i < n; i++ {
			if !hasEdge[nodes[i].ID] {
				continue
			}
			for j := i + 1; j < n; j++ {
				if !hasEdge[nodes[j].ID] {
					continue
				}
				dx := pos[i].x - pos[j].x
				dy := pos[i].y - pos[j].y
				d := math.Sqrt(dx*dx + dy*dy)
				if d >= minDist || d < 0.001 {
					continue
				}
				push := (minDist - d) / 2
				nx, ny := dx/d*push, dy/d*push
				pos[i].x += nx
				pos[i].y += ny
				pos[j].x -= nx
				pos[j].y -= ny
				moved = true
			}
		}
		if !moved {
			break
		}
	}

	// ── Position isolated nodes in a grid to the right ───────────────────
	var isoNodes []int
	for i, nd := range nodes {
		if !hasEdge[nd.ID] {
			isoNodes = append(isoNodes, i)
		}
	}
	if len(isoNodes) > 0 {
		// Compute bounding box of connected nodes to place grid just outside.
		var maxX float64 = -math.MaxFloat64
		for i, nd := range nodes {
			if hasEdge[nd.ID] && pos[i].x > maxX {
				maxX = pos[i].x
			}
		}
		if maxX == -math.MaxFloat64 {
			maxX = 0
		}
		cols := int(math.Ceil(math.Sqrt(float64(len(isoNodes)))))
		if cols < 1 {
			cols = 1
		}
		step := nodeRadius * 2.5
		originX := maxX + nodeRadius*8
		originY := -float64(len(isoNodes)/cols) * step / 2
		for idx, ni := range isoNodes {
			col := idx % cols
			row := idx / cols
			pos[ni] = vec2{
				x: originX + float64(col)*step,
				y: originY + float64(row)*step,
			}
		}
	}

	// ── Persist positions ────────────────────────────────────────────────
	positions := make(map[string]Position, n)
	for i, nd := range nodes {
		positions[nd.ID] = Position{X: pos[i].x, Y: pos[i].y}
	}
	return l.SetLayout(positions)
}

// SetLayout stores the given positions in the database.
func (l *LatticeDB) SetLayout(positions map[string]Position) error {
	return l.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketLayout)
		for id, p := range positions {
			data, err := json.Marshal(p)
			if err != nil {
				return err
			}
			if err := b.Put([]byte(id), data); err != nil {
				return err
			}
		}
		return nil
	})
}

// GetLayout returns all stored node positions.
// Returns an empty map (not an error) if no layout has been computed yet.
func (l *LatticeDB) GetLayout() (map[string]Position, error) {
	out := make(map[string]Position)
	err := l.db.View(func(tx *bbolt.Tx) error {
		return tx.Bucket(bucketLayout).ForEach(func(k, v []byte) error {
			var p Position
			if err := json.Unmarshal(v, &p); err != nil {
				return err
			}
			out[string(k)] = p
			return nil
		})
	})
	return out, err
}

// HasLayout returns true when at least one position has been stored.
func (l *LatticeDB) HasLayout() (bool, error) {
	var has bool
	err := l.db.View(func(tx *bbolt.Tx) error {
		c := tx.Bucket(bucketLayout).Cursor()
		k, _ := c.First()
		has = k != nil
		return nil
	})
	return has, err
}
