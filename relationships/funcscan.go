// Package funcscan finds every mention of a function inside a project
// directory using tree-sitter.
//
// The function is identified either by name or by the file and line range
// of its definition. Each mention is classified: a direct call, the function
// being passed as an argument, another kind of code reference, a mention in
// a comment or string literal, or a mention in a non-code file such as
// documentation. For mentions located inside another function, that
// function's name and line range are reported as well.
package funcscan

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Kind tells how the function is mentioned.
type Kind string

const (
	// KindCall is a direct call: foo(), obj.foo(), pkg::foo().
	KindCall Kind = "call"
	// KindArgument means the function itself is passed to another call:
	// run(foo), map(&foo), list.forEach(this::foo).
	KindArgument Kind = "argument"
	// KindReference is any other use in code: assignment, import,
	// decorator, struct field value, forward declaration, ...
	KindReference Kind = "reference"
	// KindDefinition is a definition of a function with this name
	// (another overload, an interface method, an implementation, ...).
	KindDefinition Kind = "definition"
	// KindComment is a mention inside a source code comment.
	KindComment Kind = "comment"
	// KindString is a mention inside a string literal.
	KindString Kind = "string"
	// KindText is a mention in a non-code file (README, docs, configs).
	KindText Kind = "text"
)

// Function describes a function definition. Lines are 1-based, inclusive.
type Function struct {
	Name      string `json:"name"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
}

// Reference is a single mention of the searched function.
type Reference struct {
	// Kind is how the function is mentioned.
	Kind Kind `json:"kind"`
	// File is the path relative to the scanned root.
	File string `json:"file"`
	// Language is the detected language, empty for plain text files.
	Language string `json:"language,omitempty"`
	// Line and Column are 1-based; Column counts characters, not bytes.
	Line   int `json:"line"`
	Column int `json:"column"`
	// Text is the trimmed source line containing the mention.
	Text string `json:"text"`
	// Enclosing is the named function containing the mention, nil when the
	// mention is at file level or in a non-code file.
	Enclosing *Function `json:"enclosing,omitempty"`
}

// Target identifies the function to look for. Set either Name, or File
// together with StartLine and EndLine (1-based, inclusive).
type Target struct {
	Name      string
	File      string
	StartLine int
	EndLine   int
}

// Options tunes a scan. The zero value (or nil) uses the defaults.
type Options struct {
	// SkipDirs are directory names that are not descended into.
	// nil means DefaultSkipDirs.
	SkipDirs []string
	// MaxFileSize is the largest file that is read, in bytes.
	// 0 means 2 MiB.
	MaxFileSize int64
	// CodeOnly disables the search in non-code (text) files.
	CodeOnly bool
}

// DefaultSkipDirs is used when Options.SkipDirs is nil.
var DefaultSkipDirs = []string{
	".git", ".hg", ".svn", ".idea", ".vscode", ".gradle",
	"node_modules", "vendor", "__pycache__", "target", "dist", "build",
}

// FindByName finds all mentions of the function called name under root.
func FindByName(root, name string) ([]Reference, error) {
	return Find(root, Target{Name: name}, nil)
}

// FindByLines finds all mentions of the function defined in file between
// startLine and endLine. file may be absolute or relative to root.
// The definition itself is not included in the result.
func FindByLines(root, file string, startLine, endLine int) ([]Reference, error) {
	return Find(root, Target{File: file, StartLine: startLine, EndLine: endLine}, nil)
}

// Resolve returns the function defined in file at the given lines. An exact
// match of the range wins; otherwise the function lying inside the range, or
// the innermost function containing it, is used.
func Resolve(file string, startLine, endLine int) (Function, error) {
	fn, _, err := resolve(file, startLine, endLine)
	return fn, err
}

// Find is the general entry point behind FindByName and FindByLines.
func Find(root string, t Target, opts *Options) ([]Reference, error) {
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

	if info, err := os.Stat(root); err != nil {
		return nil, err
	} else if !info.IsDir() {
		return nil, fmt.Errorf("funcscan: %s is not a directory", root)
	}

	// Work out the name, and which definition to leave out of the result.
	name := strings.TrimSpace(t.Name)
	var ownFile string
	ownOffset := -1
	if name == "" {
		if t.File == "" {
			return nil, errors.New("funcscan: target needs a Name or a File with lines")
		}
		file := t.File
		if _, err := os.Stat(file); err != nil && !filepath.IsAbs(file) {
			file = filepath.Join(root, file)
		}
		fn, off, err := resolve(file, t.StartLine, t.EndLine)
		if err != nil {
			return nil, err
		}
		name, ownOffset = fn.Name, off
		ownFile, _ = filepath.Abs(file)
	}

	var refs []Reference
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable entries are skipped, not fatal
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
		if info, err := d.Info(); err != nil || info.Size() > maxSize {
			return nil
		}
		l := registry()[strings.ToLower(filepath.Ext(path))]
		if l == nil && opts.CodeOnly {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil || !containsWord(src, name) {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			rel = path
		}
		if l == nil {
			refs = append(refs, scanText(rel, src, name)...)
			return nil
		}
		own := -1
		if ownOffset >= 0 {
			if abs, _ := filepath.Abs(path); abs == ownFile {
				own = ownOffset
			}
		}
		found, err := scanCode(l, rel, src, name, own)
		if err != nil {
			return nil
		}
		refs = append(refs, found...)
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.SliceStable(refs, func(i, j int) bool {
		a, b := refs[i], refs[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Column < b.Column
	})
	return refs, nil
}
