// Package bash contains the Bash language rules for funcscan.
package bash

import (
	sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_bash "github.com/tree-sitter/tree-sitter-bash/bindings/go"

	"github.com/notrobot1/funcscan/lang"
)

// Spec returns the Bash language description.
//
// A command invocation is a call; a function name used as a word argument
// of another command (`trap cleanup EXIT`, `xargs -I{} foo`) is an argument.
func Spec() *lang.Spec {
	return &lang.Spec{
		Name:        "bash",
		Extensions:  []string{".sh", ".bash"},
		Grammar:     sitter.NewLanguage(tree_sitter_bash.Language()),
		Identifiers: []string{"word"},
		Comments:    []string{"comment"},
		Strings:     []string{"string", "raw_string", "heredoc_body"},
		Calls: []lang.CallRule{
			{Kind: "command", Callee: "name", Args: "argument"},
		},
		FuncName: lang.NameByField(map[string]string{
			"function_definition": "name",
		}),
	}
}
