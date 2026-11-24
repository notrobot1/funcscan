// Package rust contains the Rust language rules for funcscan.
package rust

import (
	sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_rust "github.com/tree-sitter/tree-sitter-rust/bindings/go"

	"github.com/notrobot1/funcscan/relationships/lang"
)

// Spec returns the Rust language description.
func Spec() *lang.Spec {
	return &lang.Spec{
		Name:        "rust",
		Extensions:  []string{".rs"},
		Grammar:     sitter.NewLanguage(tree_sitter_rust.Language()),
		Identifiers: []string{"identifier", "field_identifier"},
		Comments:    []string{"line_comment", "block_comment"},
		Strings:     []string{"string_literal", "raw_string_literal"},
		Calls: []lang.CallRule{
			{Kind: "call_expression", Callee: "function", Args: "arguments"},
		},
		FuncName: lang.NameByField(map[string]string{
			"function_item":           "name",
			"function_signature_item": "name", // trait methods
		}),
	}
}
