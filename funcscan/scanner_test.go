package funcscan

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLanguages(t *testing.T) {
	cases := []struct {
		file string
		src  string
		want []string
	}{
		{"a.go", "package a\nfunc Foo() {}\ntype T struct{}\nfunc (t *T) Bar() {}\n", []string{"Foo", "Bar"}},
		{"a.py", "def foo():\n    pass\nclass A:\n    async def bar(self):\n        pass\n", []string{"foo", "bar"}},
		{"a.js", "function foo() {}\nconst bar = () => 1;\nclass A { baz() {} qux = () => 2 }\nexports.h = function () {};\nfunction* gen() {}\nconst o = { m: () => 1 };\n",
			[]string{"foo", "bar", "baz", "qux", "h", "gen", "m"}},
		{"a.ts", "function foo(a: number): void {}\nconst bar = async (x: string) => x;\nclass A { private baz(): void {} qux = () => 2 }\n",
			[]string{"foo", "bar", "baz", "qux"}},
		{"a.tsx", "export function App() { return <div/>; }\nconst Btn = () => <b/>;\n", []string{"App", "Btn"}},
		{"A.java", "class A { A() {} void foo() {} static int bar(int x) { return x; } }\n", []string{"A", "foo", "bar"}},
		{"a.c", "int proto(void);\nint foo(void) { return 0; }\nstatic char **bar(int a) { return 0; }\n", []string{"foo", "bar"}},
		{"a.cpp", "class A { public: void decl(); int inl() { return 1; } ~A() {} };\nvoid A::decl() {}\ntemplate <typename T> T& ident(T& t) { return t; }\nnamespace n { int f() { return 0; } }\n",
			[]string{"inl", "~A", "A::decl", "ident", "f"}},
		{"a.cs", "class A { public A() {} public void Foo() { int L() { return 1; } } }\n", []string{"A", "Foo", "L"}},
		{"a.kt", "fun foo() {}\nclass A { fun bar(x: Int): Int = x }\nfun String.ext() {}\n", []string{"foo", "bar", "ext"}},
		{"a.lua", "function foo() end\nlocal function bar() end\nfunction M.baz() end\nfunction M:qux() end\n", []string{"foo", "bar", "M.baz", "M:qux"}},
		{"a.php", "<?php\nfunction foo() {}\nclass A { public function bar() {} }\n", []string{"foo", "bar"}},
		{"a.rb", "def foo; end\nclass A\n  def bar; end\n  def self.baz; end\nend\n", []string{"foo", "bar", "baz"}},
		{"a.rs", "fn foo() {}\nstruct S;\nimpl S { pub fn bar(&self) {} }\ntrait T { fn sig(&self); }\n", []string{"foo", "bar"}},
		{"a.scala", "object A {\n  def foo(x: Int): Int = x\n  def bar() = {}\n}\n", []string{"foo", "bar"}},
		{"a.sh", "foo() { echo 1; }\nfunction bar { echo 2; }\n", []string{"foo", "bar"}},
	}

	s, err := New()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			funcs, err := s.ScanSource(context.Background(), tc.file, []byte(tc.src))
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, f := range funcs {
				got = append(got, f.Name)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestScanGroupsByDirectory(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"main.go":                 "package main\nfunc main() {}\n",
		"api/handler.py":          "def handle():\n    pass\n",
		"api/util.js":             "function helper() {}\n",
		"core/engine/run.rs":      "fn run() {}\n",
		"node_modules/x/index.js": "function ignored() {}\n",
		".hidden/x.go":            "package x\nfunc Ignored() {}\n",
		"docs/readme.md":          "# no code\n",
	}
	for name, src := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	res, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string][]string{}
	for _, m := range res.Modules {
		for _, f := range m.Functions {
			got[m.Path] = append(got[m.Path], f.Language+":"+f.Name)
		}
	}
	want := map[string][]string{
		".":           {"go:main"},
		"api":         {"python:handle", "javascript:helper"},
		"core/engine": {"rust:run"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if len(res.Errors) != 0 {
		t.Errorf("unexpected errors: %v", res.Errors)
	}
}
