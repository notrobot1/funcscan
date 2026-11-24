package main

import "fmt"

func main() {
	result, err := runPipeline("input.csv")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println("result:", result)
}

func runPipeline(path string) (string, error) {
	raw, err := loadFile(path)
	if err != nil {
		return "", err
	}
	validated, err := validate(raw)
	if err != nil {
		return "", err
	}
	return transform(validated), nil
}

func loadFile(path string) ([]byte, error) {
	return []byte("data from " + path), nil
}

func validate(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty data")
	}
	return sanitize(data), nil
}

func sanitize(data []byte) []byte {
	return data
}

func transform(data []byte) string {
	return encode(string(data))
}

func encode(s string) string {
	return "[" + s + "]"
}
