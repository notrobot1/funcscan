// Package lang describes the contract every language package implements.
//
// A language is described declaratively by a Spec: which tree-sitter node
// kinds are identifiers, comments, strings and calls, and how to obtain the
// name of a function definition. The generic engine in the root package does
// the rest.
package lang

import (
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// CallRule describes one "call-like" node kind of a grammar.
type CallRule struct {
	// Kind is the tree-sitter node kind (e.g. "call_expression").
	Kind string
	// Callee is the field name holding the called expression.
	// Empty means "the first named child".
	Callee string
	// Args is the field name holding the arguments (a list node, or the
	// argument itself when the grammar repeats the field, as bash does).
	Args string
	// ArgKinds are kinds of argument-list nodes that are direct children of
	// the call without a field name (used by grammars without fields).
	ArgKinds []string
}

// Spec is a full description of one language.
type Spec struct {
	// Name is a human readable language name ("go", "python", ...).
	Name string
	// Extensions are lower-case file extensions including the dot.
	Extensions []string
	// Grammar is the tree-sitter language.
	Grammar *sitter.Language
	// Identifiers are node kinds whose text may be a function name.
	Identifiers []string
	// Comments are comment node kinds.
	Comments []string
	// Strings are string literal node kinds.
	Strings []string
	// Calls describes call-like nodes.
	Calls []CallRule
	// FuncName returns the name node of n if n is a (named) function
	// definition, otherwise nil.
	FuncName func(n *sitter.Node, src []byte) *sitter.Node
	// Normalize optionally rewrites identifier text before comparison
	// (e.g. Ruby symbol ":foo" -> "foo").
	Normalize func(kind, text string) string
}

// NameByField builds a FuncName implementation for grammars where every
// function-like node stores its name in a field: kind -> field name.
func NameByField(rules map[string]string) func(*sitter.Node, []byte) *sitter.Node {
	return func(n *sitter.Node, _ []byte) *sitter.Node {
		field, ok := rules[n.Kind()]
		if !ok {
			return nil
		}
		return n.ChildByFieldName(field)
	}
}

// Descend follows the given fields (tried in order on every step) until a
// node of one of the stop kinds is reached. Useful for nested declarators
// and qualified names. Returns nil when the chain breaks.
func Descend(n *sitter.Node, stop []string, fields ...string) *sitter.Node {
	for depth := 0; n != nil && depth < 32; depth++ {
		for _, k := range stop {
			if n.Kind() == k {
				return n
			}
		}
		var next *sitter.Node
		for _, f := range fields {
			if next = n.ChildByFieldName(f); next != nil {
				break
			}
		}
		n = next
	}
	return nil
}
