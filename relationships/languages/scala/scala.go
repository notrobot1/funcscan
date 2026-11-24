// Package scala contains the Scala language rules for funcscan.
package scala

import (
	sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_scala "github.com/tree-sitter/tree-sitter-scala/bindings/go"

	"github.com/notrobot1/funcscan/relationships/lang"
)

// Spec returns the Scala language description.
func Spec() *lang.Spec {
	return &lang.Spec{
		Name:        "scala",
		Extensions:  []string{".scala", ".sc"},
		Grammar:     sitter.NewLanguage(tree_sitter_scala.Language()),
		Identifiers: []string{"identifier", "operator_identifier"},
		Comments:    []string{"comment", "block_comment"},
		Strings:     []string{"string", "interpolated_string_expression"},
		Calls: []lang.CallRule{
			{Kind: "call_expression", Callee: "function", Args: "arguments"},
			// a foo b  ==  a.foo(b)
			{Kind: "infix_expression", Callee: "operator", Args: "right"},
		},
		FuncName: lang.NameByField(map[string]string{
			"function_definition":  "name",
			"function_declaration": "name",
		}),
	}
}
