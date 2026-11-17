// Команда funcscan печатает JSON со всеми функциями проекта,
// сгруппированными по папкам.
//
//	funcscan [-o out.json] [-ignore dir1,dir2] [путь]
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/example/funcscan"
)

func main() {
	out := flag.String("o", "", "файл для записи JSON (по умолчанию stdout)")
	ignore := flag.String("ignore", "", "папки для пропуска через запятую (заменяет список по умолчанию)")
	flag.Parse()

	root := "."
	if flag.NArg() > 0 {
		root = flag.Arg(0)
	}

	var opts []funcscan.Option
	if *ignore != "" {
		opts = append(opts, funcscan.WithIgnoreDirs(strings.Split(*ignore, ",")...))
	}
	scanner, err := funcscan.New(opts...)
	if err != nil {
		fatal(err)
	}
	res, err := scanner.Scan(context.Background(), root)
	if err != nil {
		fatal(err)
	}

	w := os.Stdout
	if *out != "" {
		f, err := os.Create(*out)
		if err != nil {
			fatal(err)
		}
		defer f.Close()
		w = f
	}
	if err := res.WriteJSON(w); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "funcscan:", err)
	os.Exit(1)
}
