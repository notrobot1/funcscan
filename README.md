# funcscan

Использование:

```
res, err := funcscan.Scan("./my-project")
if err != nil { ... }
res.WriteJSON(os.Stdout)
```

Или из командной строки: `go run ./cmd/funcscan -o out.json ./my-project`. Результат:

```json
{
  "root": "./my-project",
  "modules": [
    {
      "path": "auth",
      "functions": [
        { "name": "Login", "file": "auth/auth.go", "line": 2, "language": "go" },
        { "name": "hash_password", "file": "auth/crypto.py", "line": 1, "language": "python" }
      ]
    }
  ]
}
```
