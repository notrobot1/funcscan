// Package c contains the C language rules for funcscan.
package c

import (
	sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_c "github.com/tree-sitter/tree-sitter-c/bindings/go"

	"github.com/notrobot1/funcscan/relationships/lang"
)

// funcName unwraps the declarator chain of a function_definition:
// pointer_declarator -> function_declarator -> identifier.
func funcName(n *sitter.Node, _ []byte) *sitter.Node {
	if n.Kind() != "function_definition" {
		return nil
	}
	return lang.Descend(n.ChildByFieldName("declarator"), []string{"identifier"}, "declarator")
}

// Spec returns the C language description.
func Spec() *lang.Spec {
	return &lang.Spec{
		Name:        "c",
		Extensions:  []string{".c"},
		Grammar:     sitter.NewLanguage(tree_sitter_c.Language()),
		Identifiers: []string{"identifier", "field_identifier"},
		Comments:    []string{"comment"},
		Strings:     []string{"string_literal"},
		Calls: []lang.CallRule{
			{Kind: "call_expression", Callee: "function", Args: "arguments"},
		},
		FuncName: funcName,
	}
}
