// Package lua contains the Lua language rules for funcscan.
package lua

import (
	tree_sitter_lua "github.com/tree-sitter-grammars/tree-sitter-lua/bindings/go"
	sitter "github.com/tree-sitter/go-tree-sitter"

	"github.com/notrobot1/funcscan/relationships/lang"
)

// lastIdent returns the final identifier of `a`, `a.b.c` or `a:b`.
func lastIdent(n *sitter.Node) *sitter.Node {
	return lang.Descend(n, []string{"identifier"}, "field", "method")
}

// funcName handles `function foo()`, `function M.foo()`, `function M:foo()`
// and `foo = function() end` / `M.foo = function() end`.
func funcName(n *sitter.Node, _ []byte) *sitter.Node {
	switch n.Kind() {
	case "function_declaration":
		return lastIdent(n.ChildByFieldName("name"))
	case "assignment_statement":
		var vars, exprs *sitter.Node
		for i := uint(0); i < n.NamedChildCount(); i++ {
			switch c := n.NamedChild(i); c.Kind() {
			case "variable_list":
				vars = c
			case "expression_list":
				exprs = c
			}
		}
		if vars == nil || exprs == nil || vars.NamedChildCount() != 1 || exprs.NamedChildCount() != 1 {
			return nil
		}
		if exprs.NamedChild(0).Kind() != "function_definition" {
			return nil
		}
		return lastIdent(vars.NamedChild(0))
	}
	return nil
}

// Spec returns the Lua language description.
func Spec() *lang.Spec {
	return &lang.Spec{
		Name:        "lua",
		Extensions:  []string{".lua"},
		Grammar:     sitter.NewLanguage(tree_sitter_lua.Language()),
		Identifiers: []string{"identifier"},
		Comments:    []string{"comment"},
		Strings:     []string{"string"},
		Calls: []lang.CallRule{
			{Kind: "function_call", Callee: "name", Args: "arguments"},
		},
		FuncName: funcName,
	}
}
