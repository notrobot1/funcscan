package funcgraph

import (
	"encoding/json"
	"fmt"
	"os"

	bbolt "go.etcd.io/bbolt"
)

var (
	bucketNodes     = []byte("nodes")
	bucketEdges     = []byte("edges")
	bucketOutIdx    = []byte("out_idx")     // from → []to  (callees index)
	bucketInIdx     = []byte("in_idx")      // to   → []from (callers index)
	bucketScanMeta  = []byte("_scan_meta")  // scan progress metadata
	bucketScanDone  = []byte("_scan_done")  // set of processed function names
	bucketLayout    = []byte("_layout")     // nodeId → JSON Position
	bucketConfig    = []byte("_config")     // arbitrary JSON config (saved tabs, etc.)
)

// scanStatus values stored in bucketScanMeta under key "status".
const (
	scanStatusScanning = "scanning"
	scanStatusDone     = "done"
)

// ScanState describes the persisted state of an in-progress or completed scan.
type ScanState struct {
	Status string // "scanning" | "done" | "" (never started)
	Phase  string // "1" (definitions) | "2" (edges)
	Root   string // absolute root directory of the scan
}

// LatticeDB is an embedded graph database backed by bbolt.
// It stores function nodes and their call-dependency edges.
type LatticeDB struct {
	db   *bbolt.DB
	temp string // non-empty for memory-only instances
}

// Open opens or creates a persistent LatticeDB at path.
func Open(path string) (*LatticeDB, error) {
	db, err := bbolt.Open(path, 0600, nil)
	if err != nil {
		return nil, fmt.Errorf("funcgraph: open db: %w", err)
	}
	l := &LatticeDB{db: db}
	if err := l.initBuckets(); err != nil {
		db.Close()
		return nil, err
	}
	return l, nil
}

// OpenMemory opens a temporary in-memory LatticeDB.
// The backing file is deleted automatically on Close.
func OpenMemory() (*LatticeDB, error) {
	f, err := os.CreateTemp("", "funcgraph-*.db")
	if err != nil {
		return nil, fmt.Errorf("funcgraph: temp file: %w", err)
	}
	path := f.Name()
	f.Close()

	db, err := bbolt.Open(path, 0600, nil)
	if err != nil {
		os.Remove(path)
		return nil, fmt.Errorf("funcgraph: open db: %w", err)
	}
	l := &LatticeDB{db: db, temp: path}
	if err := l.initBuckets(); err != nil {
		db.Close()
		os.Remove(path)
		return nil, err
	}
	return l, nil
}

func (l *LatticeDB) initBuckets() error {
	return l.db.Update(func(tx *bbolt.Tx) error {
		for _, name := range [][]byte{
			bucketNodes, bucketEdges, bucketOutIdx, bucketInIdx,
			bucketScanMeta, bucketScanDone, bucketLayout, bucketConfig,
		} {
			if _, err := tx.CreateBucketIfNotExists(name); err != nil {
				return fmt.Errorf("funcgraph: create bucket %s: %w", name, err)
			}
		}
		return nil
	})
}

// ── Scan-state helpers ───────────────────────────────────────────────────────

// ScanStateGet returns the current persisted scan state.
func (l *LatticeDB) ScanStateGet() (ScanState, error) {
	var s ScanState
	err := l.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketScanMeta)
		get := func(k string) string {
			v := b.Get([]byte(k))
			if v == nil {
				return ""
			}
			return string(v)
		}
		s.Status = get("status")
		s.Phase  = get("phase")
		s.Root   = get("root")
		return nil
	})
	return s, err
}

// scanStateSet writes the scan state inside a write transaction.
func (l *LatticeDB) scanStateSet(status, phase, root string) error {
	return l.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketScanMeta)
		for k, v := range map[string]string{
			"status": status,
			"phase":  phase,
			"root":   root,
		} {
			if err := b.Put([]byte(k), []byte(v)); err != nil {
				return err
			}
		}
		return nil
	})
}

// ScanStateClear removes all scan-progress metadata and processed-name records.
// Call this before a fresh scan.
func (l *LatticeDB) ScanStateClear() error {
	return l.db.Update(func(tx *bbolt.Tx) error {
		if err := tx.DeleteBucket(bucketScanMeta); err != nil && err != bbolt.ErrBucketNotFound {
			return err
		}
		if err := tx.DeleteBucket(bucketScanDone); err != nil && err != bbolt.ErrBucketNotFound {
			return err
		}
		for _, name := range [][]byte{bucketScanMeta, bucketScanDone} {
			if _, err := tx.CreateBucket(name); err != nil {
				return err
			}
		}
		return nil
	})
}

// scanMarkDone records that a function name has been processed in Phase 2.
func (l *LatticeDB) scanMarkDone(name string) error {
	return l.db.Update(func(tx *bbolt.Tx) error {
		return tx.Bucket(bucketScanDone).Put([]byte(name), []byte{1})
	})
}

// scanIsDone returns the full set of already-processed function names.
func (l *LatticeDB) scanIsDone() (map[string]struct{}, error) {
	out := make(map[string]struct{})
	err := l.db.View(func(tx *bbolt.Tx) error {
		return tx.Bucket(bucketScanDone).ForEach(func(k, _ []byte) error {
			out[string(k)] = struct{}{}
			return nil
		})
	})
	return out, err
}

// Close closes the database, removing the backing file for memory instances.
func (l *LatticeDB) Close() error {
	err := l.db.Close()
	if l.temp != "" {
		os.Remove(l.temp)
	}
	return err
}

// Path returns the filesystem path of the database file.
func (l *LatticeDB) Path() string { return l.db.Path() }

// AddFunction inserts or replaces a function node.
func (l *LatticeDB) AddFunction(fn FuncNode) error {
	data, err := json.Marshal(fn)
	if err != nil {
		return err
	}
	return l.db.Update(func(tx *bbolt.Tx) error {
		return tx.Bucket(bucketNodes).Put([]byte(fn.ID), data)
	})
}

// AddEdge inserts a dependency edge. The edge's ID is derived from From, To,
// and Kind when empty. Duplicate edges are silently ignored.
func (l *LatticeDB) AddEdge(e Edge) error {
	if e.ID == "" {
		e.ID = e.From + "→" + e.To + ":" + string(e.Kind)
	}
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return l.db.Update(func(tx *bbolt.Tx) error {
		eb := tx.Bucket(bucketEdges)
		if eb.Get([]byte(e.ID)) != nil {
			return nil
		}
		if err := eb.Put([]byte(e.ID), data); err != nil {
			return err
		}
		if err := appendIndex(tx.Bucket(bucketOutIdx), e.From, e.To); err != nil {
			return err
		}
		return appendIndex(tx.Bucket(bucketInIdx), e.To, e.From)
	})
}

// Function returns the node with the given ID.
func (l *LatticeDB) Function(id string) (FuncNode, error) {
	var fn FuncNode
	err := l.db.View(func(tx *bbolt.Tx) error {
		data := tx.Bucket(bucketNodes).Get([]byte(id))
		if data == nil {
			return fmt.Errorf("funcgraph: function %q not found", id)
		}
		return json.Unmarshal(data, &fn)
	})
	return fn, err
}

// Functions returns all function nodes stored in the database.
func (l *LatticeDB) Functions() ([]FuncNode, error) {
	var out []FuncNode
	err := l.db.View(func(tx *bbolt.Tx) error {
		return tx.Bucket(bucketNodes).ForEach(func(_, v []byte) error {
			var fn FuncNode
			if err := json.Unmarshal(v, &fn); err != nil {
				return err
			}
			out = append(out, fn)
			return nil
		})
	})
	return out, err
}

// AllEdges returns every edge in the database.
func (l *LatticeDB) AllEdges() ([]Edge, error) {
	var out []Edge
	err := l.db.View(func(tx *bbolt.Tx) error {
		return tx.Bucket(bucketEdges).ForEach(func(_, v []byte) error {
			var e Edge
			if err := json.Unmarshal(v, &e); err != nil {
				return err
			}
			out = append(out, e)
			return nil
		})
	})
	return out, err
}

// Callees returns all functions that funcID directly calls.
func (l *LatticeDB) Callees(funcID string) ([]FuncNode, error) {
	return l.neighbors(funcID, bucketOutIdx)
}

// Callers returns all functions that directly call funcID.
func (l *LatticeDB) Callers(funcID string) ([]FuncNode, error) {
	return l.neighbors(funcID, bucketInIdx)
}

// OutEdges returns all outgoing edges from funcID (what funcID calls).
func (l *LatticeDB) OutEdges(funcID string) ([]Edge, error) {
	return l.edgesByEndpoint(funcID, true)
}

// InEdges returns all incoming edges into funcID (who calls funcID).
func (l *LatticeDB) InEdges(funcID string) ([]Edge, error) {
	return l.edgesByEndpoint(funcID, false)
}

// DeleteFunction removes a node and all edges that touch it.
func (l *LatticeDB) DeleteFunction(id string) error {
	return l.db.Update(func(tx *bbolt.Tx) error {
		if err := tx.Bucket(bucketNodes).Delete([]byte(id)); err != nil {
			return err
		}
		eb := tx.Bucket(bucketEdges)
		var dead [][]byte
		eb.ForEach(func(k, v []byte) error { //nolint:errcheck
			var e Edge
			if json.Unmarshal(v, &e) == nil && (e.From == id || e.To == id) {
				dead = append(dead, append([]byte{}, k...))
			}
			return nil
		})
		for _, k := range dead {
			if err := eb.Delete(k); err != nil {
				return err
			}
		}
		tx.Bucket(bucketOutIdx).Delete([]byte(id)) //nolint:errcheck
		tx.Bucket(bucketInIdx).Delete([]byte(id))  //nolint:errcheck
		return nil
	})
}

// --- internal helpers -------------------------------------------------------

func (l *LatticeDB) neighbors(funcID string, idxBucket []byte) ([]FuncNode, error) {
	ids, err := l.indexIDs(funcID, idxBucket)
	if err != nil {
		return nil, err
	}
	out := make([]FuncNode, 0, len(ids))
	for _, id := range ids {
		fn, err := l.Function(id)
		if err != nil {
			continue
		}
		out = append(out, fn)
	}
	return out, nil
}

func (l *LatticeDB) edgesByEndpoint(funcID string, outgoing bool) ([]Edge, error) {
	var out []Edge
	err := l.db.View(func(tx *bbolt.Tx) error {
		return tx.Bucket(bucketEdges).ForEach(func(_, v []byte) error {
			var e Edge
			if err := json.Unmarshal(v, &e); err != nil {
				return nil
			}
			if (outgoing && e.From == funcID) || (!outgoing && e.To == funcID) {
				out = append(out, e)
			}
			return nil
		})
	})
	return out, err
}

func (l *LatticeDB) indexIDs(key string, bucket []byte) ([]string, error) {
	var ids []string
	err := l.db.View(func(tx *bbolt.Tx) error {
		data := tx.Bucket(bucket).Get([]byte(key))
		if data == nil {
			return nil
		}
		return json.Unmarshal(data, &ids)
	})
	return ids, err
}

// appendIndex appends value to the JSON []string stored at key in b (deduped).
func appendIndex(b *bbolt.Bucket, key, value string) error {
	var ids []string
	if data := b.Get([]byte(key)); data != nil {
		if err := json.Unmarshal(data, &ids); err != nil {
			return err
		}
	}
	for _, id := range ids {
		if id == value {
			return nil
		}
	}
	ids = append(ids, value)
	data, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	return b.Put([]byte(key), data)
}

// GetConfig returns the raw JSON stored under key (empty slice if not set).
func (l *LatticeDB) GetConfig(key string) ([]byte, error) {
	var data []byte
	err := l.db.View(func(tx *bbolt.Tx) error {
		v := tx.Bucket(bucketConfig).Get([]byte(key))
		if v != nil {
			data = append([]byte{}, v...)
		}
		return nil
	})
	return data, err
}

// SetConfig stores raw JSON under key.
func (l *LatticeDB) SetConfig(key string, value []byte) error {
	return l.db.Update(func(tx *bbolt.Tx) error {
		return tx.Bucket(bucketConfig).Put([]byte(key), value)
	})
}
