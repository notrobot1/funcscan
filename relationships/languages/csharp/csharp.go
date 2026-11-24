// Package csharp contains the C# language rules for funcscan.
package csharp

import (
	sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_c_sharp "github.com/tree-sitter/tree-sitter-c-sharp/bindings/go"

	"github.com/notrobot1/funcscan/relationships/lang"
)

// Spec returns the C# language description.
func Spec() *lang.Spec {
	return &lang.Spec{
		Name:        "csharp",
		Extensions:  []string{".cs"},
		Grammar:     sitter.NewLanguage(tree_sitter_c_sharp.Language()),
		Identifiers: []string{"identifier"},
		Comments:    []string{"comment"},
		Strings: []string{
			"string_literal", "verbatim_string_literal",
			"raw_string_literal", "interpolated_string_expression",
		},
		Calls: []lang.CallRule{
			{Kind: "invocation_expression", Callee: "function", Args: "arguments"},
		},
		FuncName: lang.NameByField(map[string]string{
			"method_declaration":       "name",
			"local_function_statement": "name",
			"constructor_declaration":  "name",
		}),
	}
}
