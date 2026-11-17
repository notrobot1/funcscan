package funcscan

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/bash"
	"github.com/smacker/go-tree-sitter/c"
	"github.com/smacker/go-tree-sitter/cpp"
	"github.com/smacker/go-tree-sitter/csharp"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/java"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/kotlin"
	"github.com/smacker/go-tree-sitter/lua"
	"github.com/smacker/go-tree-sitter/php"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/ruby"
	"github.com/smacker/go-tree-sitter/rust"
	"github.com/smacker/go-tree-sitter/scala"
	"github.com/smacker/go-tree-sitter/swift"
	"github.com/smacker/go-tree-sitter/typescript/tsx"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
)

// Имена захватов (captures), которые понимает сканер.
const (
	// captureName — узел, текст которого и есть имя функции.
	captureName = "name"
	// captureDecl — C-подобный декларатор. Имя функции может быть завёрнуто
	// в указатели/ссылки (int **foo(void)), поэтому сканер спускается по полю
	// "declarator" до самого вложенного узла.
	captureDecl = "decl"
)

// Language описывает один поддерживаемый язык: грамматику tree-sitter
// и запрос, который находит определения функций.
type Language struct {
	// Name — имя языка, попадает в JSON (поле "language").
	Name string

	grammar *sitter.Language
	pattern string

	once  sync.Once
	query *sitter.Query
	err   error
}

// compiled компилирует запрос один раз и переиспользует его.
func (l *Language) compiled() (*sitter.Query, error) {
	l.once.Do(func() {
		l.query, l.err = sitter.NewQuery([]byte(l.pattern), l.grammar)
		if l.err != nil {
			l.err = fmt.Errorf("funcscan: запрос для языка %s: %w", l.Name, l.err)
		}
	})
	return l.query, l.err
}

// Запросы для JavaScript и TypeScript почти одинаковы, отличается только
// узел поля класса.
const ecmaScriptFunctions = `
(function_declaration name: (identifier) @name)
(generator_function_declaration name: (identifier) @name)
(method_definition name: (_) @name)
(variable_declarator
  name: (identifier) @name
  value: [(arrow_function) (function_expression) (generator_function)])
(assignment_expression
  left: (identifier) @name
  right: [(arrow_function) (function_expression) (generator_function)])
(assignment_expression
  left: (member_expression property: (property_identifier) @name)
  right: [(arrow_function) (function_expression) (generator_function)])
(pair
  key: (property_identifier) @name
  value: [(arrow_function) (function_expression) (generator_function)])
`

const javascriptFunctions = ecmaScriptFunctions + `
(field_definition
  property: (property_identifier) @name
  value: [(arrow_function) (function_expression)])
`

const typescriptFunctions = ecmaScriptFunctions + `
(public_field_definition
  name: (property_identifier) @name
  value: [(arrow_function) (function_expression)])
`

var (
	langBash = &Language{Name: "bash", grammar: bash.GetLanguage(), pattern: `
(function_definition name: (word) @name)
`}
	langC = &Language{Name: "c", grammar: c.GetLanguage(), pattern: `
(function_definition declarator: (_) @decl)
`}
	langCPP = &Language{Name: "cpp", grammar: cpp.GetLanguage(), pattern: `
(function_definition declarator: (_) @decl)
`}
	langCSharp = &Language{Name: "csharp", grammar: csharp.GetLanguage(), pattern: `
(method_declaration name: (identifier) @name)
(constructor_declaration name: (identifier) @name)
(local_function_statement name: (identifier) @name)
`}
	langGo = &Language{Name: "go", grammar: golang.GetLanguage(), pattern: `
(function_declaration name: (identifier) @name)
(method_declaration name: (field_identifier) @name)
`}
	langJava = &Language{Name: "java", grammar: java.GetLanguage(), pattern: `
(method_declaration name: (identifier) @name)
(constructor_declaration name: (identifier) @name)
`}
	langJavaScript = &Language{Name: "javascript", grammar: javascript.GetLanguage(), pattern: javascriptFunctions}
	langKotlin     = &Language{Name: "kotlin", grammar: kotlin.GetLanguage(), pattern: `
(function_declaration (simple_identifier) @name)
`}
	langLua = &Language{Name: "lua", grammar: lua.GetLanguage(), pattern: `
(function_statement name: (_) @name)
`}
	langPHP = &Language{Name: "php", grammar: php.GetLanguage(), pattern: `
(function_definition name: (name) @name)
(method_declaration name: (name) @name)
`}
	langPython = &Language{Name: "python", grammar: python.GetLanguage(), pattern: `
(function_definition name: (identifier) @name)
`}
	langRuby = &Language{Name: "ruby", grammar: ruby.GetLanguage(), pattern: `
(method name: (_) @name)
(singleton_method name: (_) @name)
`}
	langRust = &Language{Name: "rust", grammar: rust.GetLanguage(), pattern: `
(function_item name: (identifier) @name)
`}
	langScala = &Language{Name: "scala", grammar: scala.GetLanguage(), pattern: `
(function_definition name: (_) @name)
`}
	langSwift = &Language{Name: "swift", grammar: swift.GetLanguage(), pattern: `
(function_declaration name: (simple_identifier) @name)
(init_declaration "init" @name)
`}
	langTypeScript = &Language{Name: "typescript", grammar: typescript.GetLanguage(), pattern: typescriptFunctions}
	// TSX — отдельная грамматика, но для пользователя это тот же TypeScript.
	langTSX = &Language{Name: "typescript", grammar: tsx.GetLanguage(), pattern: typescriptFunctions}
)

// byName — языки по имени (для WithExtension / LanguageByName).
var byName = map[string]*Language{
	"bash": langBash, "c": langC, "cpp": langCPP, "csharp": langCSharp,
	"go": langGo, "golang": langGo, "java": langJava, "javascript": langJavaScript,
	"kotlin": langKotlin, "lua": langLua, "php": langPHP, "python": langPython,
	"ruby": langRuby, "rust": langRust, "scala": langScala, "swift": langSwift,
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
	".swift": langSwift,
	".ts":    langTypeScript, ".mts": langTypeScript, ".cts": langTypeScript,
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
