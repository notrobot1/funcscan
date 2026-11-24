// Package golang contains the Go language rules for funcscan.
package golang

import (
	sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_go "github.com/tree-sitter/tree-sitter-go/bindings/go"

	"github.com/notrobot1/funcscan/lang"
)

// Spec returns the Go language description.
func Spec() *lang.Spec {
	return &lang.Spec{
		Name:        "go",
		Extensions:  []string{".go"},
		Grammar:     sitter.NewLanguage(tree_sitter_go.Language()),
		Identifiers: []string{"identifier", "field_identifier"},
		Comments:    []string{"comment"},
		Strings:     []string{"interpreted_string_literal", "raw_string_literal"},
		Calls: []lang.CallRule{
			{Kind: "call_expression", Callee: "function", Args: "arguments"},
		},
		FuncName: lang.NameByField(map[string]string{
			"function_declaration": "name",
			"method_declaration":   "name",
			"method_elem":          "name", // interface methods
		}),
	}
}
