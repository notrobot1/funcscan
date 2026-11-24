package funcscan

import (
	"sort"
	"sync"

	"github.com/notrobot1/funcscan/relationships/lang"
	"github.com/notrobot1/funcscan/relationships/languages/bash"
	"github.com/notrobot1/funcscan/relationships/languages/c"
	"github.com/notrobot1/funcscan/relationships/languages/cpp"
	"github.com/notrobot1/funcscan/relationships/languages/csharp"
	"github.com/notrobot1/funcscan/relationships/languages/golang"
	"github.com/notrobot1/funcscan/relationships/languages/java"
	"github.com/notrobot1/funcscan/relationships/languages/javascript"
	"github.com/notrobot1/funcscan/relationships/languages/kotlin"
	"github.com/notrobot1/funcscan/relationships/languages/lua"
	"github.com/notrobot1/funcscan/relationships/languages/php"
	"github.com/notrobot1/funcscan/relationships/languages/python"
	"github.com/notrobot1/funcscan/relationships/languages/ruby"
	"github.com/notrobot1/funcscan/relationships/languages/rust"
	"github.com/notrobot1/funcscan/relationships/languages/scala"
	"github.com/notrobot1/funcscan/relationships/languages/typescript"
)

// language is a Spec with its lookups precomputed.
type language struct {
	spec     *lang.Spec
	idents   map[string]bool
	comments map[string]bool
	strs     map[string]bool
	calls    map[string]lang.CallRule
}

var (
	registryOnce sync.Once
	byExt        map[string]*language
)

func set(kinds []string) map[string]bool {
	m := make(map[string]bool, len(kinds))
	for _, k := range kinds {
		m[k] = true
	}
	return m
}

func registry() map[string]*language {
	registryOnce.Do(func() {
		specs := []*lang.Spec{
			bash.Spec(), c.Spec(), cpp.Spec(), csharp.Spec(), golang.Spec(),
			java.Spec(), javascript.Spec(), kotlin.Spec(), lua.Spec(),
			php.Spec(), python.Spec(), ruby.Spec(), rust.Spec(), scala.Spec(),
			typescript.Spec(), typescript.SpecTSX(),
		}
		byExt = map[string]*language{}
		for _, s := range specs {
			l := &language{
				spec:     s,
				idents:   set(s.Identifiers),
				comments: set(s.Comments),
				strs:     set(s.Strings),
				calls:    map[string]lang.CallRule{},
			}
			for _, r := range s.Calls {
				l.calls[r.Kind] = r
			}
			for _, ext := range s.Extensions {
				byExt[ext] = l
			}
		}
	})
	return byExt
}

// Languages returns the names of all supported languages.
func Languages() []string {
	seen := map[string]bool{}
	var out []string
	for _, l := range registry() {
		if !seen[l.spec.Name] {
			seen[l.spec.Name] = true
			out = append(out, l.spec.Name)
		}
	}
	sort.Strings(out)
	return out
}
