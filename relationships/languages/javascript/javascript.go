// Package javascript contains the JavaScript language rules for funcscan.
package javascript

import (
	sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_javascript "github.com/tree-sitter/tree-sitter-javascript/bindings/go"

	"github.com/notrobot1/funcscan/lang"
)

// Shared with the typescript package, whose grammar extends this one.
var (
	Identifiers = []string{
		"identifier", "property_identifier",
		"shorthand_property_identifier", "private_property_identifier",
	}
	Comments = []string{"comment"}
	Strings  = []string{"string", "template_string"}
	Calls    = []lang.CallRule{
		{Kind: "call_expression", Callee: "function", Args: "arguments"},
		{Kind: "new_expression", Callee: "constructor", Args: "arguments"},
	}
)

var named = map[string]bool{
	"function_declaration":           true,
	"generator_function_declaration": true,
	"function_expression":            true, // only when it has a name
	"method_definition":              true,
}

func isFuncValue(n *sitter.Node) bool {
	if n == nil {
		return false
	}
	switch n.Kind() {
	case "arrow_function", "function_expression", "generator_function":
		return true
	}
	return false
}

// FuncName recognises declarations, methods and functions bound to a name:
//
//	function foo() {}      class A { foo() {} }
//	const foo = () => {}   const o = { foo: function () {} }
//	this.foo = () => {}    class A { foo = () => {} }
func FuncName(n *sitter.Node, _ []byte) *sitter.Node {
	switch kind := n.Kind(); {
	case named[kind]:
		return n.ChildByFieldName("name")
	case kind == "variable_declarator":
		if name := n.ChildByFieldName("name"); name != nil && name.Kind() == "identifier" &&
			isFuncValue(n.ChildByFieldName("value")) {
			return name
		}
	case kind == "pair":
		if isFuncValue(n.ChildByFieldName("value")) {
			if key := n.ChildByFieldName("key"); key != nil && key.Kind() == "property_identifier" {
				return key
			}
		}
	case kind == "field_definition" || kind == "public_field_definition":
		if isFuncValue(n.ChildByFieldName("value")) {
			if name := n.ChildByFieldName("property"); name != nil {
				return name
			}
			return n.ChildByFieldName("name")
		}
	case kind == "assignment_expression":
		if isFuncValue(n.ChildByFieldName("right")) {
			left := n.ChildByFieldName("left")
			if left == nil {
				return nil
			}
			if left.Kind() == "identifier" {
				return left
			}
			if left.Kind() == "member_expression" {
				return left.ChildByFieldName("property")
			}
		}
	}
	return nil
}

// Spec returns the JavaScript language description.
func Spec() *lang.Spec {
	return &lang.Spec{
		Name:        "javascript",
		Extensions:  []string{".js", ".jsx", ".mjs", ".cjs"},
		Grammar:     sitter.NewLanguage(tree_sitter_javascript.Language()),
		Identifiers: Identifiers,
		Comments:    Comments,
		Strings:     Strings,
		Calls:       Calls,
		FuncName:    FuncName,
	}
}
