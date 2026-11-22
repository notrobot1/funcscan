package funcscan

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unsafe"

	tree_sitter_kotlin "github.com/tree-sitter-grammars/tree-sitter-kotlin/bindings/go"
	tree_sitter_lua "github.com/tree-sitter-grammars/tree-sitter-lua/bindings/go"
	sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_bash "github.com/tree-sitter/tree-sitter-bash/bindings/go"
	tree_sitter_c_sharp "github.com/tree-sitter/tree-sitter-c-sharp/bindings/go"
	tree_sitter_c "github.com/tree-sitter/tree-sitter-c/bindings/go"
	tree_sitter_cpp "github.com/tree-sitter/tree-sitter-cpp/bindings/go"
	tree_sitter_go "github.com/tree-sitter/tree-sitter-go/bindings/go"
	tree_sitter_java "github.com/tree-sitter/tree-sitter-java/bindings/go"
	tree_sitter_javascript "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	tree_sitter_php "github.com/tree-sitter/tree-sitter-php/bindings/go"
	tree_sitter_python "github.com/tree-sitter/tree-sitter-python/bindings/go"
	tree_sitter_ruby "github.com/tree-sitter/tree-sitter-ruby/bindings/go"
	tree_sitter_rust "github.com/tree-sitter/tree-sitter-rust/bindings/go"
	tree_sitter_scala "github.com/tree-sitter/tree-sitter-scala/bindings/go"
	tree_sitter_typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

// Имена захватов (captures), которые понимает сканер.
const (
	// captureName — узел, текст которого и есть имя функции.
	captureName = "name"
	// captureDecl — C-подобный декларатор. Имя функции может быть завёрнуто
	// в указатели/ссылки (int **foo(void)), поэтому сканер спускается по полю
	// "declarator" до самого вложенного узла.
	captureDecl = "decl"
	// captureFunc — узел всего определения функции: по нему берутся
	// границы (строки и байтовые смещения).
	captureFunc = "func"
)

// Language описывает один поддерживаемый язык: грамматику tree-sitter
// и запрос, который находит определения функций.
type Language struct {
	// Name — имя языка, попадает в JSON (поле "language").
	Name string

	grammar unsafe.Pointer // указатель на грамматику из bindings/go
	pattern string

	once  sync.Once
	lang  *sitter.Language
	query *sitter.Query
	err   error
}

// compiled создаёт язык и компилирует запрос один раз и переиспользует их.
func (l *Language) compiled() (*sitter.Language, *sitter.Query, error) {
	l.once.Do(func() {
		l.lang = sitter.NewLanguage(l.grammar)
		query, qerr := sitter.NewQuery(l.lang, l.pattern)
		if qerr != nil {
			l.err = fmt.Errorf("funcscan: запрос для языка %s: %s", l.Name, qerr.Error())
			return
		}
		l.query = query
	})
	return l.lang, l.query, l.err
}

// Запросы для JavaScript и TypeScript почти одинаковы, отличается только
// узел поля класса.
const ecmaScriptFunctions = `
(function_declaration name: (identifier) @name) @func
(generator_function_declaration name: (identifier) @name) @func
(method_definition name: (_) @name) @func
(variable_declarator
  name: (identifier) @name
  value: [(arrow_function) (function_expression) (generator_function)]) @func
(assignment_expression
  left: (identifier) @name
  right: [(arrow_function) (function_expression) (generator_function)]) @func
(assignment_expression
  left: (member_expression property: (property_identifier) @name)
  right: [(arrow_function) (function_expression) (generator_function)]) @func
(pair
  key: (property_identifier) @name
  value: [(arrow_function) (function_expression) (generator_function)]) @func
`

const javascriptFunctions = ecmaScriptFunctions + `
(field_definition
  property: (property_identifier) @name
  value: [(arrow_function) (function_expression)]) @func
`

const typescriptFunctions = ecmaScriptFunctions + `
(public_field_definition
  name: (property_identifier) @name
  value: [(arrow_function) (function_expression)]) @func
`

var (
	langBash = &Language{Name: "bash", grammar: tree_sitter_bash.Language(), pattern: `
(function_definition name: (word) @name) @func
`}
	langC = &Language{Name: "c", grammar: tree_sitter_c.Language(), pattern: `
(function_definition declarator: (_) @decl) @func
`}
	langCPP = &Language{Name: "cpp", grammar: tree_sitter_cpp.Language(), pattern: `
(function_definition declarator: (_) @decl) @func
`}
	langCSharp = &Language{Name: "csharp", grammar: tree_sitter_c_sharp.Language(), pattern: `
(method_declaration name: (identifier) @name) @func
(constructor_declaration name: (identifier) @name) @func
(local_function_statement name: (identifier) @name) @func
`}
	langGo = &Language{Name: "go", grammar: tree_sitter_go.Language(), pattern: `
(function_declaration name: (identifier) @name) @func
(method_declaration name: (field_identifier) @name) @func
`}
	langJava = &Language{Name: "java", grammar: tree_sitter_java.Language(), pattern: `
(method_declaration name: (identifier) @name) @func
(constructor_declaration name: (identifier) @name) @func
`}
	langJavaScript = &Language{Name: "javascript", grammar: tree_sitter_javascript.Language(), pattern: javascriptFunctions}
	langKotlin     = &Language{Name: "kotlin", grammar: tree_sitter_kotlin.Language(), pattern: `
(function_declaration name: (_) @name) @func
`}
	langLua = &Language{Name: "lua", grammar: tree_sitter_lua.Language(), pattern: `
(function_declaration name: (_) @name) @func
`}
	langPHP = &Language{Name: "php", grammar: tree_sitter_php.LanguagePHP(), pattern: `
(function_definition name: (name) @name) @func
(method_declaration name: (name) @name) @func
`}
	langPython = &Language{Name: "python", grammar: tree_sitter_python.Language(), pattern: `
(function_definition name: (identifier) @name) @func
`}
	langRuby = &Language{Name: "ruby", grammar: tree_sitter_ruby.Language(), pattern: `
(method name: (_) @name) @func
(singleton_method name: (_) @name) @func
`}
	langRust = &Language{Name: "rust", grammar: tree_sitter_rust.Language(), pattern: `
(function_item name: (identifier) @name) @func
`}
	langScala = &Language{Name: "scala", grammar: tree_sitter_scala.Language(), pattern: `
(function_definition name: (_) @name) @func
`}
	langTypeScript = &Language{Name: "typescript", grammar: tree_sitter_typescript.LanguageTypescript(), pattern: typescriptFunctions}
	// TSX — отдельная грамматика, но для пользователя это тот же TypeScript.
	langTSX = &Language{Name: "typescript", grammar: tree_sitter_typescript.LanguageTSX(), pattern: typescriptFunctions}
)

// byName — языки по имени (для WithExtension / LanguageByName).
var byName = map[string]*Language{
	"bash": langBash, "c": langC, "cpp": langCPP, "csharp": langCSharp,
	"go": langGo, "golang": langGo, "java": langJava, "javascript": langJavaScript,
	"kotlin": langKotlin, "lua": langLua, "php": langPHP, "python": langPython,
	"ruby": langRuby, "rust": langRust, "scala": langScala,
	"typescript": langTypeScript, "tsx": langTSX,
}

// byExtension — определение языка по расширению файла (в нижнем регистре).
var byExtension = map[string]*Language{
	".sh": langBash, ".bash": langBash,
	".c": langC, ".h": langC,
	".cpp": langCPP, ".cc": langCPP, ".cxx": langCPP, ".c++": langCPP,
	".hpp": langCPP, ".hh": langCPP, ".hxx": langCPP, ".h++": langCPP,
	".cs":   langCSharp,
	".go":   langGo,
	".java": langJava,
	".js":   langJavaScript, ".mjs": langJavaScript, ".cjs": langJavaScript, ".jsx": langJavaScript,
	".kt": langKotlin, ".kts": langKotlin,
	".lua": langLua,
	".php": langPHP,
	".py":  langPython, ".pyi": langPython,
	".rb": langRuby, ".rake": langRuby, ".gemspec": langRuby,
	".rs":    langRust,
	".scala": langScala, ".sc": langScala,
	".ts": langTypeScript, ".mts": langTypeScript, ".cts": langTypeScript,
	".tsx": langTSX,
}

// byFilename — файлы без расширения, язык которых понятен по имени.
var byFilename = map[string]*Language{
	"Rakefile": langRuby, "Gemfile": langRuby,
	".bashrc": langBash, ".bash_profile": langBash, ".bash_aliases": langBash,
}

// DetectLanguage определяет язык по имени файла (расширению или известному
// имени вроде Rakefile). Вторым значением возвращает false, если язык
// не поддерживается.
func DetectLanguage(filename string) (*Language, bool) {
	base := filepath.Base(filename)
	if l, ok := byFilename[base]; ok {
		return l, true
	}
	// ".C" по традиции означает C++, остальные расширения регистронезависимы.
	if filepath.Ext(base) == ".C" {
		return langCPP, true
	}
	l, ok := byExtension[strings.ToLower(filepath.Ext(base))]
	return l, ok
}

// LanguageByName возвращает язык по имени ("go", "python", "cpp", ...).
func LanguageByName(name string) (*Language, bool) {
	l, ok := byName[strings.ToLower(name)]
	return l, ok
}

// SupportedLanguages возвращает отсортированный список имён поддерживаемых языков.
func SupportedLanguages() []string {
	seen := map[string]bool{}
	var out []string
	for _, l := range byName {
		if !seen[l.Name] {
			seen[l.Name] = true
			out = append(out, l.Name)
		}
	}
	sort.Strings(out)
	return out
}
