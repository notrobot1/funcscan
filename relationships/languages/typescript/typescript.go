// Package typescript contains the TypeScript and TSX rules for funcscan.
package typescript

import (
	"unsafe"

	sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"

	"github.com/notrobot1/funcscan/lang"
	"github.com/notrobot1/funcscan/languages/javascript"
)

func funcName(n *sitter.Node, src []byte) *sitter.Node {
	switch n.Kind() {
	case "function_signature", "method_signature", "abstract_method_signature":
		return n.ChildByFieldName("name")
	}
	return javascript.FuncName(n, src)
}

func spec(name string, grammar unsafe.Pointer, exts ...string) *lang.Spec {
	return &lang.Spec{
		Name:        name,
		Extensions:  exts,
		Grammar:     sitter.NewLanguage(grammar),
		Identifiers: javascript.Identifiers,
		Comments:    javascript.Comments,
		Strings:     javascript.Strings,
		Calls:       javascript.Calls,
		FuncName:    funcName,
	}
}

// Spec returns the TypeScript language description.
func Spec() *lang.Spec {
	return spec("typescript", tree_sitter_typescript.LanguageTypescript(), ".ts", ".mts", ".cts")
}

// SpecTSX returns the TSX language description.
func SpecTSX() *lang.Spec {
	return spec("tsx", tree_sitter_typescript.LanguageTSX(), ".tsx")
}
