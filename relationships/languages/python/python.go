// Package python contains the Python language rules for funcscan.
package python

import (
	sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_python "github.com/tree-sitter/tree-sitter-python/bindings/go"

	"github.com/notrobot1/funcscan/relationships/lang"
)

// Spec returns the Python language description.
func Spec() *lang.Spec {
	return &lang.Spec{
		Name:        "python",
		Extensions:  []string{".py", ".pyi"},
		Grammar:     sitter.NewLanguage(tree_sitter_python.Language()),
		Identifiers: []string{"identifier"},
		Comments:    []string{"comment"},
		Strings:     []string{"string"},
		Calls: []lang.CallRule{
			{Kind: "call", Callee: "function", Args: "arguments"},
		},
		FuncName: lang.NameByField(map[string]string{
			"function_definition": "name",
		}),
	}
}
