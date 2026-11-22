package funcscan

import (
	"encoding/json"
	"io"
)

// Function — одно найденное определение функции или метода.
type Function struct {
	Name     string `json:"name"`
	File     string `json:"file"`
	Line     int    `json:"line"`     // первая строка определения (с 1)
	EndLine  int    `json:"end_line"` // последняя строка определения
	Language string `json:"language"`

	StartByte uint32 `json:"-"` // границы определения в файле
	EndByte   uint32 `json:"-"`
}

// Module — один компонент проекта, то есть одна папка с исходниками.
type Module struct {
	Path      string     `json:"path"` // путь папки относительно корня; "." — сам корень
	Functions []Function `json:"functions"`
}

// FileError — файл, который не удалось прочитать или разобрать.
// Такие файлы не прерывают сканирование.
type FileError struct {
	File  string `json:"file"`
	Error string `json:"error"`
}

// Result — итог сканирования проекта.
type Result struct {
	Root    string      `json:"root"`
	Modules []Module    `json:"modules"`
	Errors  []FileError `json:"errors,omitempty"`
}

// JSON возвращает результат в виде отформатированного JSON.
func (r *Result) JSON() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}

// WriteJSON пишет результат в w в виде отформатированного JSON.
func (r *Result) WriteJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(r)
}
