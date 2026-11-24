// Package ruby contains the Ruby language rules for funcscan.
package ruby

import (
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_ruby "github.com/tree-sitter/tree-sitter-ruby/bindings/go"

	"github.com/notrobot1/funcscan/lang"
)

// Spec returns the Ruby language description.
//
// Note: a bare `foo` without arguments or parentheses is syntactically
// indistinguishable from a local variable in Ruby, so it is reported as a
// plain reference rather than a call.
func Spec() *lang.Spec {
	return &lang.Spec{
		Name:       "ruby",
		Extensions: []string{".rb", ".rake", ".gemspec"},
		Grammar:    sitter.NewLanguage(tree_sitter_ruby.Language()),
		// Symbols are how Ruby passes methods around: method(:foo), send(:foo).
		Identifiers: []string{"identifier", "simple_symbol", "hash_key_symbol"},
		Comments:    []string{"comment"},
		Strings:     []string{"string", "heredoc_body"},
		Calls: []lang.CallRule{
			{Kind: "call", Callee: "method", Args: "arguments"},
		},
		FuncName: lang.NameByField(map[string]string{
			"method":           "name",
			"singleton_method": "name",
		}),
		Normalize: func(kind, text string) string {
			if kind == "simple_symbol" {
				return strings.TrimPrefix(text, ":")
			}
			return text
		},
	}
}
