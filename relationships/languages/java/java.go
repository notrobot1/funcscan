// Package java contains the Java language rules for funcscan.
package java

import (
	sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_java "github.com/tree-sitter/tree-sitter-java/bindings/go"

	"github.com/notrobot1/funcscan/lang"
)

// Spec returns the Java language description.
func Spec() *lang.Spec {
	return &lang.Spec{
		Name:        "java",
		Extensions:  []string{".java"},
		Grammar:     sitter.NewLanguage(tree_sitter_java.Language()),
		Identifiers: []string{"identifier"},
		Comments:    []string{"line_comment", "block_comment"},
		Strings:     []string{"string_literal"},
		Calls: []lang.CallRule{
			{Kind: "method_invocation", Callee: "name", Args: "arguments"},
		},
		FuncName: lang.NameByField(map[string]string{
			"method_declaration":      "name",
			"constructor_declaration": "name",
		}),
	}
}
