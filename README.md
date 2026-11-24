# funcscan

Монорепозиторий из нескольких Go-модулей для анализа зависимостей между функциями в многоязычных проектах и их визуализации в браузере.

---

## Модули

| Модуль | Путь | Описание |
|---|---|---|
| `funcscan` | `funcscan/` | Сканирует исходники и возвращает список функций по папкам |
| `relationships` | `relationships/` | Находит все упоминания функции в проекте (вызовы, аргументы, определения и т.д.) |
| `funcgraph` | `funcgraph/` | Строит и хранит граф зависимостей в встроенной базе данных LatticeDB |
| `funcweb` | `funcweb/` | Веб-сервер с интерактивной визуализацией графа |
| `funcviz` | `funcviz/` | CLI-команда: сканирует проект, строит граф и поднимает сервер |

---

## Быстрый старт

```bash
cd funcviz
go build -o funcviz .

# Сканировать папку и открыть UI
./funcviz ../myproject

# Открыть http://localhost:8080 в браузере
```

---

## funcviz — CLI

```
funcviz [флаги] [путь]
```

Флаги и путь можно указывать в любом порядке.

| Флаг | По умолчанию | Описание |
|---|---|---|
| `-port` | `8080` | HTTP-порт для веб-интерфейса |
| `-host` | `localhost` | Хост для привязки |
| `-db` | `<путь>/.funcgraph.db` | Путь к файлу базы данных |
| `-skip` | — | Дополнительные папки для пропуска (через запятую) |
| `-rebuild` | — | Удалить базу и пересканировать с нуля |

### Примеры

```bash
# Сканировать текущую папку
./funcviz .

# Другой порт
./funcviz -port 9090 ./myproject

# Пропустить папки
./funcviz -skip vendor,generated ./myproject

# Принудительное пересканирование
./funcviz -rebuild ./myproject

# Своя база данных
./funcviz -db /tmp/graph.db ./myproject
```

### База данных

По умолчанию база сохраняется в `<путь>/.funcgraph.db` — рядом со сканируемым проектом. При повторном запуске той же папки она подхватывается автоматически без повторного сканирования.

---

## Возобновляемое сканирование

Если сканирование прервалось (Ctrl+C, сбой), следующий запуск **продолжит с места остановки**, а не начнёт заново.

```
# Первый запуск — прерван на середине
./funcviz ./myproject
  definitions   [██████████████████████████████] 100%  234/234
  edges         [████████░░░░░░░░░░░░░░░░░░░░░░]  27%   32/120
^C

# Повторный запуск — продолжает с 32-й функции
./funcviz ./myproject
resuming interrupted scan (234 functions already stored, phase 2)…
  definitions   [██████████████████████████████] 100%  234/234
  edges         [████████████████░░░░░░░░░░░░░░]  53%   64/120
```

Прогресс сохраняется в бакетах `_scan_meta` и `_scan_done` внутри файла базы данных после каждой обработанной функции.

Для сброса и полного пересканирования:

```bash
./funcviz -rebuild ./myproject
```

---

## Веб-интерфейс

После запуска откройте `http://localhost:8080`.

| Элемент | Описание |
|---|---|
| **▶ Layout** | Запустить/остановить физический layout (d3-force) |
| **⤢ Fit** | Вписать весь граф в экран |
| **↑ Callers / ↓ Callees** | Показать только upstream или downstream дерево выбранного узла |
| **Поиск** | Поиск по имени функции или файлу |
| **Наведение на узел** | Подсвечивает всё дерево вызовов (вверх и вниз) |
| **Клик на узел** | Открывает боковую панель с исходным кодом файла; выбранная функция подсвечена |
| **Перетаскивание края панели** | Изменение ширины панели с кодом |

### Оптимизация при большом количестве функций

При отдалении камеры (zoom out) близко расположенные узлы автоматически сливаются в один кластерный узел с подписью `название (+N)`. При приближении они раскрываются обратно. Рёбра скрываются при сильном отдалении.

---

## relationships — библиотека

```go
import funcscan "github.com/notrobot1/funcscan/relationships"

// Найти все вызовы функции
refs, err := funcscan.FindByName("/path/to/project", "myFunc")
for _, ref := range refs {
    fmt.Printf("%s:%d [%s] inside %s\n",
        ref.File, ref.Line, ref.Kind, ref.Enclosing.Name)
}

// Найти все определения функций в проекте
defs, err := funcscan.Definitions("/path/to/project", nil)
```

Поддерживаемые языки: Go, Python, JavaScript, TypeScript, Rust, Ruby, Java, C, C++, C#, Kotlin, Scala, Lua, PHP, Bash.

---

## funcgraph — библиотека

```go
import "github.com/notrobot1/funcscan/funcgraph"

// Открыть или создать базу
db, err := funcgraph.Open("graph.db")
defer db.Close()

// Сканировать проект и построить граф
err = db.ScanAndStore("/path/to/project", &funcgraph.ScanOptions{
    Progress: func(phase string, done, total int) {
        fmt.Printf("\r%s %d/%d", phase, done, total)
    },
})

// Запросить граф
callees, _ := db.Callees("main.go:5")
callers, _ := db.Callers("main.go:5")
paths, _   := db.Paths("main.go:5", "utils.go:12", 10)
sg, _      := db.Subgraph("main.go:5", 3, funcgraph.DirectionBoth)
```

---

## funcweb — библиотека

```go
import "github.com/notrobot1/funcscan/funcweb"

srv := funcweb.New(funcweb.Config{
    Port:    8080,
    Host:    "localhost",
    DB:      db,       // *funcgraph.LatticeDB
    RootDir: "/path/to/project",
})

// Graceful shutdown
go func() {
    <-sigCh
    srv.Shutdown(context.Background())
}()

srv.Start() // блокирует
```

### API endpoints

| Метод | Путь | Описание |
|---|---|---|
| `GET` | `/api/graph` | Все узлы и рёбра |
| `GET` | `/api/node/{id}` | Один узел по ID |
| `GET` | `/api/subgraph/{id}` | Окрестность узла (`?depth=3&direction=both\|callers\|callees`) |
| `GET` | `/api/file` | Содержимое файла (`?path=relative/path`) |

---

## Тестовые данные

В папке `testdata/` находятся примеры с одинаковой цепочкой вызовов на 9 языках:

```
main → runPipeline → loadFile
                   → validate → sanitize
                   → transform → encode
```

Языки: Go, Python, JavaScript, TypeScript, Rust, Ruby, Java, C++, C#.

```bash
./funcviz ../testdata
```
