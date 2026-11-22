// Package funcscan рекурсивно обходит дерево исходников, находит определения
// функций с помощью tree-sitter и группирует их по модулям (папкам).
//
// Язык каждого файла определяется по его имени, поэтому в одном проекте
// могут быть смешаны любые поддерживаемые языки.
//
//	res, err := funcscan.Scan("./my-project")
//	if err != nil { ... }
//	res.WriteJSON(os.Stdout)
package funcscan

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
)

// Scanner ищет определения функций. Нулевое значение не используется —
// создавайте сканер через New. После создания сканер не меняется, поэтому
// его можно переиспользовать и вызывать из нескольких горутин.
type Scanner struct {
	ignoreDirs  map[string]bool
	skipHidden  bool
	maxFileSize int64
	extensions  map[string]*Language // пользовательские переопределения
}

// Option настраивает Scanner.
type Option func(*Scanner) error

// WithIgnoreDirs задаёт имена папок, которые нужно пропускать
// (заменяет список по умолчанию: node_modules, vendor, __pycache__).
func WithIgnoreDirs(names ...string) Option {
	return func(s *Scanner) error {
		s.ignoreDirs = make(map[string]bool, len(names))
		for _, n := range names {
			s.ignoreDirs[n] = true
		}
		return nil
	}
}

// WithHiddenDirs включает обход скрытых папок (.git, .idea и т.п.),
// которые по умолчанию пропускаются.
func WithHiddenDirs() Option {
	return func(s *Scanner) error { s.skipHidden = false; return nil }
}

// WithMaxFileSize ограничивает размер разбираемых файлов в байтах
// (по умолчанию 2 МБ; 0 — без ограничения). Слишком большие файлы пропускаются.
func WithMaxFileSize(n int64) Option {
	return func(s *Scanner) error { s.maxFileSize = n; return nil }
}

// WithExtension привязывает расширение файла к языку, например
// WithExtension(".h", "cpp") или WithExtension(".zsh", "bash").
func WithExtension(ext, language string) Option {
	return func(s *Scanner) error {
		l, ok := LanguageByName(language)
		if !ok {
			return fmt.Errorf("funcscan: неизвестный язык %q", language)
		}
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		s.extensions[strings.ToLower(ext)] = l
		return nil
	}
}

// New создаёт сканер с заданными опциями.
func New(opts ...Option) (*Scanner, error) {
	s := &Scanner{
		ignoreDirs:  map[string]bool{"node_modules": true, "vendor": true, "__pycache__": true},
		skipHidden:  true,
		maxFileSize: 2 << 20,
		extensions:  map[string]*Language{},
	}
	for _, opt := range opts {
		if err := opt(s); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// Scan — короткий путь: сканирует root сканером с настройками по умолчанию.
func Scan(root string) (*Result, error) {
	s, err := New()
	if err != nil {
		return nil, err
	}
	return s.Scan(context.Background(), root)
}

// Scan рекурсивно обходит root и возвращает функции, сгруппированные по папкам.
// Ошибки отдельных файлов попадают в Result.Errors и не прерывают обход.
func (s *Scanner) Scan(ctx context.Context, root string) (*Result, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("funcscan: %s не является папкой", root)
	}

	res := &Result{Root: filepath.ToSlash(root), Modules: []Module{}}
	modules := map[string][]Function{}
	parser := sitter.NewParser()
	defer parser.Close()

	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = path
		}
		rel = filepath.ToSlash(rel)

		if err != nil {
			res.Errors = append(res.Errors, FileError{File: rel, Error: err.Error()})
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if path != root && s.skipDir(d.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		lang, ok := s.detect(d.Name())
		if !ok {
			return nil
		}
		if s.maxFileSize > 0 {
			if fi, err := d.Info(); err == nil && fi.Size() > s.maxFileSize {
				return nil
			}
		}

		src, err := os.ReadFile(path)
		if err != nil {
			res.Errors = append(res.Errors, FileError{File: rel, Error: err.Error()})
			return nil
		}
		funcs, err := extract(ctx, parser, lang, src)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			res.Errors = append(res.Errors, FileError{File: rel, Error: err.Error()})
			return nil
		}
		if len(funcs) == 0 {
			return nil
		}
		for i := range funcs {
			funcs[i].File = rel
		}
		dir := filepath.ToSlash(filepath.Dir(rel))
		modules[dir] = append(modules[dir], funcs...)
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	for dir, funcs := range modules {
		res.Modules = append(res.Modules, Module{Path: dir, Functions: funcs})
	}
	sort.Slice(res.Modules, func(i, j int) bool { return res.Modules[i].Path < res.Modules[j].Path })
	return res, nil
}

// ScanSource разбирает один исходник в памяти. Имя файла нужно только для
// определения языка; в результате оно записывается в поле File как есть.
func (s *Scanner) ScanSource(ctx context.Context, filename string, src []byte) ([]Function, error) {
	lang, ok := s.detect(filepath.Base(filename))
	if !ok {
		return nil, fmt.Errorf("funcscan: язык файла %s не поддерживается", filename)
	}
	parser := sitter.NewParser()
	defer parser.Close()
	funcs, err := extract(ctx, parser, lang, src)
	if err != nil {
		return nil, err
	}
	for i := range funcs {
		funcs[i].File = filepath.ToSlash(filename)
	}
	return funcs, nil
}

func (s *Scanner) skipDir(name string) bool {
	if s.ignoreDirs[name] {
		return true
	}
	return s.skipHidden && strings.HasPrefix(name, ".")
}

func (s *Scanner) detect(name string) (*Language, bool) {
	if l, ok := s.extensions[strings.ToLower(filepath.Ext(name))]; ok {
		return l, true
	}
	return DetectLanguage(name)
}

// extract разбирает исходник и возвращает найденные функции в порядке
// их появления в файле.
// extract разбирает исходник и возвращает найденные функции в порядке
// их появления в файле.
func extract(ctx context.Context, parser *sitter.Parser, lang *Language, src []byte) ([]Function, error) {
	query, err := lang.compiled()
	if err != nil {
		return nil, err
	}
	parser.SetLanguage(lang.grammar)
	tree, err := parser.ParseCtx(ctx, nil, src)
	if err != nil {
		return nil, err
	}
	defer tree.Close()

	cursor := sitter.NewQueryCursor()
	defer cursor.Close()
	cursor.Exec(query, tree.RootNode())

	var funcs []Function
	seen := map[uint32]bool{} // один и тот же узел могут поймать несколько шаблонов
	
for {
	match, ok := cursor.NextMatch()
	if !ok {
		break
	}
	var nameNode, funcNode *sitter.Node
	for _, capture := range match.Captures {
		switch query.CaptureNameForId(capture.Index) {
		case captureFunc:
			funcNode = capture.Node
		case captureDecl:
			nameNode = innermostDeclarator(capture.Node)
		default:
			nameNode = capture.Node
		}
	}
	if nameNode == nil || seen[nameNode.StartByte()] {
		continue
	}
	name := strings.TrimSpace(nameNode.Content(src))
	if name == "" {
		continue
	}
	if funcNode == nil {
		funcNode = nameNode
	}
	seen[nameNode.StartByte()] = true
	funcs = append(funcs, Function{
		Name:      name,
		Line:      int(funcNode.StartPoint().Row) + 1,
		EndLine:   int(funcNode.EndPoint().Row) + 1,
		StartByte: funcNode.StartByte(),
		EndByte:   funcNode.EndByte(),
		Language:  lang.Name,
	})
}
	sort.SliceStable(funcs, func(i, j int) bool { return funcs[i].Line < funcs[j].Line })
	return funcs, nil
}

// innermostDeclarator спускается по вложенным деклараторам C/C++
// (pointer_declarator → function_declarator → identifier) до узла с именем.
// Возвращает nil, если это не декларатор функции (например, ошибка разбора).
func innermostDeclarator(n *sitter.Node) *sitter.Node {
	isFunction := false
	for n != nil {
		if n.Type() == "function_declarator" {
			isFunction = true
		}
		next := n.ChildByFieldName("declarator")
		if next == nil {
			// У этих обёрток нет поля "declarator": скобки (int (*f(void))(int))
			// и ссылки C++ (T& f()).
			t := n.Type()
			if (t == "parenthesized_declarator" || t == "reference_declarator") && n.NamedChildCount() > 0 {
				n = n.NamedChild(0)
				continue
			}
			break
		}
		n = next
	}
	if !isFunction {
		return nil
	}
	return n
}
