// Package kotlin contains the Kotlin language rules for funcscan.
package kotlin

import (
	tree_sitter_kotlin "github.com/tree-sitter-grammars/tree-sitter-kotlin/bindings/go"
	sitter "github.com/tree-sitter/go-tree-sitter"

	"github.com/notrobot1/funcscan/relationships/lang"
)

// Spec returns the Kotlin language description.
func Spec() *lang.Spec {
	return &lang.Spec{
		Name:        "kotlin",
		Extensions:  []string{".kt", ".kts"},
		Grammar:     sitter.NewLanguage(tree_sitter_kotlin.Language()),
		Identifiers: []string{"identifier", "simple_identifier"},
		Comments:    []string{"line_comment", "block_comment", "multiline_comment"},
		Strings:     []string{"string_literal", "multiline_string_literal"},
		Calls: []lang.CallRule{
			// The grammar has no fields here: the callee is the first
			// named child, the arguments live in value_arguments.
			{Kind: "call_expression", ArgKinds: []string{"value_arguments", "call_suffix"}},
		},
		FuncName: func(n *sitter.Node, _ []byte) *sitter.Node {
			if n.Kind() != "function_declaration" {
				return nil
			}
			if name := n.ChildByFieldName("name"); name != nil {
				return name
			}
			// Older grammar revisions: first simple_identifier child.
			for i := uint(0); i < n.NamedChildCount(); i++ {
				if c := n.NamedChild(i); c.Kind() == "simple_identifier" {
					return c
				}
			}
			return nil
		},
	}
}
