package funcweb

// analyze.go — ленивый анализ функций при первом просмотре кода.
//
// Архитектура:
//   - AnalyzeFunction — единственная точка входа; вызывается хэндлером при
//     открытии файла. Запускает все зарегистрированные анализаторы по порядку.
//   - Каждый анализатор — отдельная функция с сигнатурой analyzerFunc.
//   - Результат (булево решение + score) кэшируется в БД, чтобы не
//     переанализировать функцию при повторных открытиях.
//   - Если нужно добавить новый анализатор — добавьте функцию и зарегистрируйте
//     её в слайсе analyzers в init().
//
// Пример добавления анализатора:
//
//	func analyzeSecretLeak(ctx analyzeCtx) (AnalysisResult, error) {
//	    // ...
//	}
//
//	func init() { analyzers = append(analyzers, analyzeSecretLeak) }

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/notrobot1/anti_entry_points/inputclf"
	"github.com/notrobot1/funcscan/funcgraph"
)

// AnalysisResult is the result of all analyzers for a single function.
type AnalysisResult struct {
	IsEntryPoint bool    `json:"is_entry_point"`
	EntryScore   float64 `json:"entry_score"`
	FunctionName string  `json:"function_name"`
}

// analyzeCtx carries all data needed by individual analyzers.
type analyzeCtx struct {
	Node    funcgraph.FuncNode
	Src     []byte // raw source file content
	RootDir string
}

// analyzerFunc is the signature every analyzer must implement.
type analyzerFunc func(analyzeCtx) (AnalysisResult, error)

// analyzers is the ordered list of passes run by AnalyzeFunction.
// Add new analyzers here; they run in order and their results are merged.
var analyzers []analyzerFunc

func init() {
	analyzers = append(analyzers,
		analyzeEntryPoint, // detects user-input entry points via ML classifier
	)
}

// AnalyzeFunction runs all analysis passes on fn, caches the result in the DB,
// and—if the function is an entry point—adds it to the "Entry Points" saved tab.
//
// It is designed to be called lazily when the user first views a function's
// source, so the heavyweight analysis does not block the initial graph scan.
// Subsequent calls for the same nodeID return immediately from cache.
func (s *Server) AnalyzeFunction(fn funcgraph.FuncNode) (AnalysisResult, error) {
	cacheKey := "analysis:" + fn.ID

	// Return cached result if available.
	if cached, err := s.cfg.DB.GetConfig(cacheKey); err == nil && len(cached) > 0 {
		var r AnalysisResult
		if json.Unmarshal(cached, &r) == nil {
			return r, nil
		}
	}

	// Read source file once; share it across all analyzers.
	src, err := os.ReadFile(filepath.Join(s.cfg.RootDir, fn.File))
	if err != nil {
		return AnalysisResult{}, fmt.Errorf("analyze: read %s: %w", fn.File, err)
	}

	ctx := analyzeCtx{Node: fn, Src: src, RootDir: s.cfg.RootDir}

	// Run all registered analyzers; merge results.
	var merged AnalysisResult
	for _, a := range analyzers {
		r, err := a(ctx)
		if err != nil {
			continue // non-fatal: log and move on
		}
		mergeResult(&merged, r)
	}

	// Persist to cache.
	if data, err := json.Marshal(merged); err == nil {
		s.cfg.DB.SetConfig(cacheKey, data) //nolint:errcheck
	}

	// If this is an entry point, add it to the "Entry Points" saved tab.
	if merged.IsEntryPoint {
		if err := s.addToEntryPoints(fn.ID); err != nil {
			return merged, fmt.Errorf("analyze: add entry point: %w", err)
		}
	}

	return merged, nil
}

// mergeResult folds a single analyzer result into the accumulated result.
func mergeResult(dst *AnalysisResult, src AnalysisResult) {
	if src.IsEntryPoint {
		dst.IsEntryPoint = true
	}
	if src.EntryScore > dst.EntryScore {
		dst.EntryScore = src.EntryScore
	}
	if src.FunctionName != "" && dst.FunctionName == "" {
		dst.FunctionName = src.FunctionName
	}
}

// addToEntryPoints adds nodeID to the "Entry Points" tab in the saved DB,
// creating the structure if it does not exist yet.
func (s *Server) addToEntryPoints(nodeID string) error {
	raw, _ := s.cfg.DB.GetConfig("saved_tabs")

	var data struct {
		Tabs []struct {
			ID        string   `json:"id"`
			Name      string   `json:"name"`
			Permanent bool     `json:"permanent"`
			Nodes     []string `json:"nodes"`
		} `json:"tabs"`
	}

	if len(raw) > 0 {
		json.Unmarshal(raw, &data) //nolint:errcheck
	}

	// Ensure the entry-points tab exists.
	epIdx := -1
	for i, t := range data.Tabs {
		if t.ID == "entry-points" {
			epIdx = i
			break
		}
	}
	if epIdx < 0 {
		data.Tabs = append([]struct {
			ID        string   `json:"id"`
			Name      string   `json:"name"`
			Permanent bool     `json:"permanent"`
			Nodes     []string `json:"nodes"`
		}{{ID: "entry-points", Name: "Entry Points", Permanent: true}}, data.Tabs...)
		epIdx = 0
	}

	// Dedup.
	for _, id := range data.Tabs[epIdx].Nodes {
		if id == nodeID {
			return nil // already there
		}
	}
	data.Tabs[epIdx].Nodes = append(data.Tabs[epIdx].Nodes, nodeID)

	encoded, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return s.cfg.DB.SetConfig("saved_tabs", encoded)
}

// AnalyzeAll runs AnalyzeFunction on every node that has not been cached yet.
// It is designed to be called once at startup before the HTTP server starts.
// progress(done, total) is called after each function; may be nil.
func (s *Server) AnalyzeAll(progress func(done, total int)) error {
	if s.cfg.RootDir == "" {
		return nil
	}

	// Skip if every node already has a cached result.
	nodes, err := s.cfg.DB.Functions()
	if err != nil {
		return err
	}

	// Collect nodes that have not been analyzed yet.
	pending := nodes[:0]
	for _, fn := range nodes {
		cached, _ := s.cfg.DB.GetConfig("analysis:" + fn.ID)
		if len(cached) == 0 {
			pending = append(pending, fn)
		}
	}
	if len(pending) == 0 {
		return nil
	}

	total := len(pending)
	for i, fn := range pending {
		s.AnalyzeFunction(fn) //nolint:errcheck
		if progress != nil {
			progress(i+1, total)
		}
	}
	return nil
}

// ── Analyzers ────────────────────────────────────────────────────────────────

// analyzeEntryPoint uses the ML classifier from anti_entry_points to decide
// whether the function accepts user-controlled input (probability > 0.5).
func analyzeEntryPoint(ctx analyzeCtx) (AnalysisResult, error) {
	res, err := inputclf.PredictLines(ctx.Src, ctx.Node.File, ctx.Node.StartLine, ctx.Node.EndLine)
	if err != nil {
		return AnalysisResult{}, fmt.Errorf("inputclf: %w", err)
	}
	return AnalysisResult{
		IsEntryPoint: res.AcceptsInput,
		EntryScore:   res.Probability,
		FunctionName: res.Function,
	}, nil
}
