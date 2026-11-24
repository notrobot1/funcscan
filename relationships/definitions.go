package funcscan

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// DefinedFunction is a named function definition found in a source file.
type DefinedFunction struct {
	Name      string
	File      string // relative to root
	StartLine int
	EndLine   int
	Language  string
}

// Definitions returns all named function definitions found under root.
// It honours the same Options as Find (SkipDirs, MaxFileSize).
func Definitions(root string, opts *Options) ([]DefinedFunction, error) {
	if opts == nil {
		opts = &Options{}
	}
	skipDirs := opts.SkipDirs
	if skipDirs == nil {
		skipDirs = DefaultSkipDirs
	}
	skip := set(skipDirs)
	maxSize := opts.MaxFileSize
	if maxSize <= 0 {
		maxSize = 2 << 20
	}

	var defs []DefinedFunction
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != root && skip[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, err2 := d.Info()
		if err2 != nil || info.Size() > maxSize {
			return nil
		}
		l := registry()[strings.ToLower(filepath.Ext(path))]
		if l == nil {
			return nil
		}
		src, err2 := os.ReadFile(path)
		if err2 != nil {
			return nil
		}
		tree, err2 := parse(l, src)
		if err2 != nil {
			return nil
		}
		defer tree.Close()

		rel, err2 := filepath.Rel(root, path)
		if err2 != nil {
			rel = path
		}

		var cands []candidate
		collectFuncs(l, tree.RootNode(), src, &cands)
		for _, c := range cands {
			defs = append(defs, DefinedFunction{
				Name:      c.fn.Name,
				File:      rel,
				StartLine: c.fn.StartLine,
				EndLine:   c.fn.EndLine,
				Language:  l.spec.Name,
			})
		}
		return nil
	})
	return defs, err
}
