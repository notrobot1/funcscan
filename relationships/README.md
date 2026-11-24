# funcscan

Go-библиотека на tree-sitter: находит все упоминания функции в папке проекта
и сообщает, в каком виде функция упоминается.

```
go get github.com/notrobot1/funcscan
```

Нужен cgo (грамматики tree-sitter написаны на C).

## Использование

```go
// по имени
refs, err := funcscan.FindByName("./project", "handleRequest")

// по файлу и строкам определения (1-based, включительно);
// само определение в результат не попадает
refs, err := funcscan.FindByLines("./project", "internal/api/server.go", 42, 80)

for _, r := range refs {
    fmt.Printf("%s:%d:%d %s", r.File, r.Line, r.Column, r.Kind)
    if r.Enclosing != nil {
        fmt.Printf(" in %s (%d-%d)", r.Enclosing.Name, r.Enclosing.StartLine, r.Enclosing.EndLine)
    }
    fmt.Println()
}
```

Общая точка входа с настройками: `funcscan.Find(root, funcscan.Target{...}, &funcscan.Options{...})`.
`funcscan.Resolve(file, start, end)` отдельно возвращает функцию по строкам.

## Виды упоминаний (`Reference.Kind`)

| Kind         | Что это                                                            |
|--------------|--------------------------------------------------------------------|
| `call`       | прямой вызов: `foo()`, `obj.foo()`, `pkg::foo()`                   |
| `argument`   | функция передаётся аргументом: `run(foo)`, `run(&foo)`, `run(this::foo)`, `method(:foo)` |
| `reference`  | другое использование в коде: присваивание, импорт, декоратор, прототип |
| `definition` | определение функции с таким именем (перегрузка, реализация интерфейса) |
| `comment`    | упоминание в комментарии                                           |
| `string`     | упоминание в строковом литерале                                    |
| `text`       | упоминание в не-кодовом файле (README, документация, конфиги)      |

Для каждого упоминания возвращаются файл, строка, колонка, текст строки и,
если оно находится внутри именованной функции, — её имя и строки начала/конца
(`Enclosing`). Анонимные функции пропускаются: берётся ближайшая именованная.

## Языки

Bash, C, C++, C#, Go, Java, JavaScript, Kotlin, Lua, PHP, Python, Ruby, Rust,
Scala, TypeScript (+TSX). Один язык — одна папка в `languages/`:

```
funcscan.go        публичный API
scan.go            общий движок (обход дерева, классификация)
registry.go        подключение языков
lang/              контракт языка (lang.Spec)
languages/<язык>/  правила конкретного языка
cmd/funcscan/      CLI, печатает результат в JSON
```

Чтобы добавить язык, создайте папку с функцией `Spec() *lang.Spec`
и добавьте её в `registry.go`.

## Ограничения

Поиск синтаксический, по имени: типы и области видимости не разрешаются,
поэтому одноимённые функции/методы разных классов не различаются.
