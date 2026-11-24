// Command funcscan prints every mention of a function as JSON.
//
//	funcscan -root ./project -name handleRequest
//	funcscan -root ./project -file internal/api/server.go -start 42 -end 80
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/notrobot1/funcscan"
)

func main() {
	root := flag.String("root", ".", "project directory to scan")
	name := flag.String("name", "", "function name")
	file := flag.String("file", "", "file with the function definition (instead of -name)")
	start := flag.Int("start", 0, "first line of the function (with -file)")
	end := flag.Int("end", 0, "last line of the function (with -file)")
	codeOnly := flag.Bool("code-only", false, "do not search non-code files")
	flag.Parse()

	refs, err := funcscan.Find(*root,
		funcscan.Target{Name: *name, File: *file, StartLine: *start, EndLine: *end},
		&funcscan.Options{CodeOnly: *codeOnly})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(refs); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
