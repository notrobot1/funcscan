package funcscan

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

// ---- word search -----------------------------------------------------------

func isWordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// wordOffsets returns the byte offsets of every whole-word occurrence of
// name in text.
func wordOffsets(text []byte, name string) []int {
	var out []int
	needle := []byte(name)
	for from := 0; from <= len(text)-len(needle); {
		i := bytes.Index(text[from:], needle)
		if i < 0 {
			break
		}
		start := from + i
		end := start + len(needle)
		before, _ := utf8.DecodeLastRune(text[:start])
		after, _ := utf8.DecodeRune(text[end:])
		if (start == 0 || !isWordRune(before)) && (end == len(text) || !isWordRune(after)) {
			out = append(out, start)
		}
		from = start + 1
	}
	return out
}

func containsWord(text []byte, name string) bool {
	return len(wordOffsets(text, name)) > 0
}

// ---- positions -------------------------------------------------------------

type lineIndex struct {
	src    []byte
	starts []int
}

func newLineIndex(src []byte) *lineIndex {
	starts := []int{0}
	for i, b := range src {
		if b == '\n' {
			starts = append(starts, i+1)
		}
	}
	return &lineIndex{src: src, starts: starts}
}

// at returns the 1-based line and column of a byte offset plus the line text.
func (li *lineIndex) at(off int) (line, col int, text string) {
	i := sort.SearchInts(li.starts, off+1) - 1
	start := li.starts[i]
	end := len(li.src)
	if i+1 < len(li.starts) {
		end = li.starts[i+1]
	}
	text = strings.TrimSpace(string(li.src[start:end]))
	if r := []rune(text); len(r) > 240 {
		text = string(r[:240]) + "…"
	}
	return i + 1, utf8.RuneCount(li.src[start:off]) + 1, text
}

// ---- plain text files ------------------------------------------------------

func isBinary(src []byte) bool {
	if len(src) > 8000 {
		src = src[:8000]
	}
	return bytes.IndexByte(src, 0) >= 0
}

func scanText(file string, src []byte, name string) []Reference {
	if isBinary(src) {
		return nil
	}
	li := newLineIndex(src)
	var refs []Reference
	for _, off := range wordOffsets(src, name) {
		line, col, text := li.at(off)
		refs = append(refs, Reference{Kind: KindText, File: file, Line: line, Column: col, Text: text})
	}
	return refs
}

// ---- source files ----------------------------------------------------------

func parse(l *language, src []byte) (*sitter.Tree, error) {
	parser := sitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(l.spec.Grammar); err != nil {
		return nil, fmt.Errorf("funcscan: %s grammar: %w", l.spec.Name, err)
	}
	tree := parser.Parse(src, nil)
	if tree == nil {
		return nil, fmt.Errorf("funcscan: %s parse failed", l.spec.Name)
	}
	return tree, nil
}

type walker struct {
	l    *language
	src  []byte
	name string
	file string
	own  int // byte offset of the target's own name node, -1 if none
	li   *lineIndex
	seen map[int]bool
	refs []Reference
}

func scanCode(l *language, file string, src []byte, name string, own int) ([]Reference, error) {
	tree, err := parse(l, src)
	if err != nil {
		return nil, err
	}
	defer tree.Close()
	w := &walker{l: l, src: src, name: name, file: file, own: own,
		li: newLineIndex(src), seen: map[int]bool{}}
	w.walk(tree.RootNode())
	return w.refs, nil
}

func same(a, b *sitter.Node) bool {
	return a.StartByte() == b.StartByte() && a.EndByte() == b.EndByte()
}

func (w *walker) text(n *sitter.Node) string {
	t := string(w.src[n.StartByte():n.EndByte()])
	if w.l.spec.Normalize != nil {
		t = w.l.spec.Normalize(n.Kind(), t)
	}
	return t
}

func (w *walker) walk(n *sitter.Node) {
	kind := n.Kind()
	switch {
	case w.l.comments[kind]:
		w.mentions(n, KindComment)
		return
	case w.l.strs[kind]:
		// Interpolated code first, then whatever is left is literal text.
		w.children(n)
		w.mentions(n, KindString)
		return
	case w.l.idents[kind] && w.text(n) == w.name:
		off := int(n.StartByte()) + strings.Index(string(w.src[n.StartByte():n.EndByte()]), w.name)
		k := w.classify(n)
		if !(k == KindDefinition && int(n.StartByte()) == w.own) {
			w.add(k, off, w.enclosing(n, k == KindDefinition))
		}
		w.seen[off] = true
	}
	w.children(n)
}

func (w *walker) children(n *sitter.Node) {
	for i := uint(0); i < n.ChildCount(); i++ {
		if c := n.Child(i); c != nil {
			w.walk(c)
		}
	}
}

func (w *walker) mentions(n *sitter.Node, kind Kind) {
	var enc *Function
	resolved := false
	for _, rel := range wordOffsets(w.src[n.StartByte():n.EndByte()], w.name) {
		off := int(n.StartByte()) + rel
		if w.seen[off] {
			continue
		}
		if !resolved {
			enc, resolved = w.enclosing(n, false), true
		}
		w.add(kind, off, enc)
	}
}

func (w *walker) add(kind Kind, off int, enc *Function) {
	w.seen[off] = true
	line, col, text := w.li.at(off)
	w.refs = append(w.refs, Reference{
		Kind: kind, File: w.file, Language: w.l.spec.Name,
		Line: line, Column: col, Text: text, Enclosing: enc,
	})
}

// enclosing returns the nearest named function around n. When n is itself
// the name of a definition, that definition is skipped.
func (w *walker) enclosing(n *sitter.Node, isDefName bool) *Function {
	for a := n.Parent(); a != nil; a = a.Parent() {
		name := w.l.spec.FuncName(a, w.src)
		if name == nil {
			continue
		}
		if isDefName && same(name, n) {
			continue
		}
		return &Function{
			Name:      w.text(name),
			StartLine: int(a.StartPosition().Row) + 1,
			EndLine:   int(a.EndPosition().Row) + 1,
		}
	}
	return nil
}

var (
	// What may follow the name inside a callee / argument: nothing, or
	// explicit type arguments such as foo<T>, foo::<T>, foo[T].
	tailRe = regexp.MustCompile(`^\s*(?:(?:::)?<[^()]*>|\[[^()]*\])?\s*$`)
	// What may precede the name inside an argument for it to still be
	// "the function itself": a qualifier (pkg., Self::, this::, obj->), an
	// address-of / block marker (&, *, :), or a named-argument label.
	argPrefixRe = regexp.MustCompile(`^(?:[\p{L}\p{N}_]+\s*[=:]\s*)?[\p{L}\p{N}_.:&*$@\\>?-]*$`)
)

func childContaining(parent, x *sitter.Node) (uint, *sitter.Node) {
	for i := uint(0); i < parent.ChildCount(); i++ {
		c := parent.Child(i)
		if c != nil && c.StartByte() <= x.StartByte() && x.EndByte() <= c.EndByte() {
			return i, c
		}
	}
	return 0, nil
}

func has(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// classify decides how identifier x (already known to match) is used.
func (w *walker) classify(x *sitter.Node) Kind {
	for a := x.Parent(); a != nil; a = a.Parent() {
		if name := w.l.spec.FuncName(a, w.src); name != nil {
			if same(name, x) {
				return KindDefinition
			}
			return KindReference
		}
		rule, ok := w.l.calls[a.Kind()]
		if !ok {
			continue
		}
		idx, d := childContaining(a, x)
		if d == nil {
			return KindReference
		}
		field := a.FieldNameForChild(uint32(idx))

		isCallee := rule.Callee != "" && field == rule.Callee
		if rule.Callee == "" {
			first := a.NamedChild(0)
			isCallee = first != nil && same(first, d)
		}
		if isCallee {
			if tailRe.Match(w.src[x.EndByte():d.EndByte()]) {
				return KindCall
			}
			return KindReference // e.g. the receiver in foo.bar()
		}

		byField := rule.Args != "" && field == rule.Args
		if !byField && !has(rule.ArgKinds, d.Kind()) {
			return KindReference
		}
		// Step from the argument list down to the single argument.
		arg := d
		if byField && !same(arg, x) {
			if _, c := childContaining(arg, x); c != nil {
				arg = c
			}
		}
		for has(rule.ArgKinds, arg.Kind()) && !same(arg, x) {
			_, c := childContaining(arg, x)
			if c == nil {
				break
			}
			arg = c
		}
		if argPrefixRe.Match(w.src[arg.StartByte():x.StartByte()]) &&
			tailRe.Match(w.src[x.EndByte():arg.EndByte()]) {
			return KindArgument
		}
		return KindReference // part of a larger expression: f(x + foo)
	}
	return KindReference
}

// ---- resolving a function by its lines -------------------------------------

type candidate struct {
	fn     Function
	offset int
}

func collectFuncs(l *language, n *sitter.Node, src []byte, out *[]candidate) {
	if name := l.spec.FuncName(n, src); name != nil {
		text := string(src[name.StartByte():name.EndByte()])
		if l.spec.Normalize != nil {
			text = l.spec.Normalize(name.Kind(), text)
		}
		*out = append(*out, candidate{
			fn: Function{
				Name:      text,
				StartLine: int(n.StartPosition().Row) + 1,
				EndLine:   int(n.EndPosition().Row) + 1,
			},
			offset: int(name.StartByte()),
		})
	}
	for i := uint(0); i < n.ChildCount(); i++ {
		if c := n.Child(i); c != nil {
			collectFuncs(l, c, src, out)
		}
	}
}

// resolve returns the function at the given lines and the byte offset of its
// name node.
func resolve(file string, start, end int) (Function, int, error) {
	if start <= 0 || end < start {
		return Function{}, 0, fmt.Errorf("funcscan: invalid line range %d-%d", start, end)
	}
	l := registry()[strings.ToLower(filepath.Ext(file))]
	if l == nil {
		return Function{}, 0, fmt.Errorf("funcscan: unsupported file type: %s", file)
	}
	src, err := os.ReadFile(file)
	if err != nil {
		return Function{}, 0, err
	}
	tree, err := parse(l, src)
	if err != nil {
		return Function{}, 0, err
	}
	defer tree.Close()

	var all []candidate
	collectFuncs(l, tree.RootNode(), src, &all)

	var inside, around *candidate
	for i := range all {
		c := &all[i]
		switch {
		case c.fn.StartLine == start && c.fn.EndLine == end:
			return c.fn, c.offset, nil
		case c.fn.StartLine >= start && c.fn.EndLine <= end:
			// The range is wider than the function (doc comment,
			// decorators, attributes): take the outermost one.
			if inside == nil {
				inside = c
			}
		case c.fn.StartLine <= start && c.fn.EndLine >= end:
			// The range is a part of the function: take the innermost.
			if around == nil || c.fn.EndLine-c.fn.StartLine <= around.fn.EndLine-around.fn.StartLine {
				around = c
			}
		}
	}
	if inside != nil {
		return inside.fn, inside.offset, nil
	}
	if around != nil {
		return around.fn, around.offset, nil
	}
	return Function{}, 0, errors.New("funcscan: no function found at the given lines")
}
