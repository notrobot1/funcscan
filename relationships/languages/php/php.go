// Package php contains the PHP language rules for funcscan.
package php

import (
	sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_php "github.com/tree-sitter/tree-sitter-php/bindings/go"

	"github.com/notrobot1/funcscan/lang"
)

// Spec returns the PHP language description.
//
// PHP callables are often plain strings ('foo', [$this, 'foo']); those are
// reported as string mentions.
func Spec() *lang.Spec {
	return &lang.Spec{
		Name:        "php",
		Extensions:  []string{".php", ".phtml"},
		Grammar:     sitter.NewLanguage(tree_sitter_php.LanguagePHP()),
		Identifiers: []string{"name"},
		Comments:    []string{"comment"},
		Strings:     []string{"string", "encapsed_string", "heredoc", "nowdoc"},
		Calls: []lang.CallRule{
			{Kind: "function_call_expression", Callee: "function", Args: "arguments"},
			{Kind: "member_call_expression", Callee: "name", Args: "arguments"},
			{Kind: "nullsafe_member_call_expression", Callee: "name", Args: "arguments"},
			{Kind: "scoped_call_expression", Callee: "name", Args: "arguments"},
		},
		FuncName: lang.NameByField(map[string]string{
			"function_definition": "name",
			"method_declaration":  "name",
		}),
	}
}
