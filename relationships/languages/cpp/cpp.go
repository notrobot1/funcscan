// Package cpp contains the C++ language rules for funcscan.
package cpp

import (
	sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_cpp "github.com/tree-sitter/tree-sitter-cpp/bindings/go"

	"github.com/notrobot1/funcscan/relationships/lang"
)

var nameKinds = []string{"identifier", "field_identifier", "destructor_name", "operator_name"}

// funcName unwraps declarators, qualified names (A::B::foo) and templates.
func funcName(n *sitter.Node, _ []byte) *sitter.Node {
	if n.Kind() != "function_definition" {
		return nil
	}
	return lang.Descend(n.ChildByFieldName("declarator"), nameKinds, "declarator", "name")
}

// Spec returns the C++ language description. Header files (.h) are parsed
// with the C++ grammar, which is a superset of C for our purposes.
func Spec() *lang.Spec {
	return &lang.Spec{
		Name:        "cpp",
		Extensions:  []string{".cpp", ".cc", ".cxx", ".hpp", ".hh", ".hxx", ".h"},
		Grammar:     sitter.NewLanguage(tree_sitter_cpp.Language()),
		Identifiers: []string{"identifier", "field_identifier"},
		Comments:    []string{"comment"},
		Strings:     []string{"string_literal", "raw_string_literal"},
		Calls: []lang.CallRule{
			{Kind: "call_expression", Callee: "function", Args: "arguments"},
		},
		FuncName: funcName,
	}
}
