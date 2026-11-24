package funcscan

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// Every sample defines `target` on known lines and a function `caller`
// that calls it, passes it as an argument, and mentions it in a comment and
// in a string.
var samples = []struct {
	file       string
	start, end int // lines of target's definition
	src        string
}{
	{"a.go", 3, 3, `package a

func target() {}

func caller() {
	// see target
	target()
	run(target)
	_ = "target here"
}
`},
	{"a.py", 1, 2, `def target():
    pass

def caller():
    # see target
    target()
    run(target)
    s = "target here"
`},
	{"a.js", 1, 1, `function target() {}

function caller() {
  // see target
  target();
  run(target);
  const s = "target here";
}
`},
	{"a.ts", 1, 1, `const target = (): void => {};

function caller(): void {
  // see target
  target();
  run(target);
  const s: string = "target here";
}
`},
	{"a.tsx", 1, 1, `function target() { return <div/>; }

function caller() {
  // see target
  target();
  run(target);
  const s = "target here";
}
`},
	{"A.java", 2, 2, `class A {
  void target() {}

  void caller() {
    // see target
    target();
    run(this::target);
    String s = "target here";
  }
}
`},
	{"a.kt", 1, 1, `fun target() {}

fun caller() {
    // see target
    target()
    run(::target)
    val s = "target here"
}
`},
	{"a.scala", 2, 2, `object A {
  def target(): Unit = {}

  def caller(): Unit = {
    // see target
    target()
    run(target)
    val s = "target here"
  }
}
`},
	{"a.c", 1, 1, `void target(void) {}

void caller(void) {
  /* see target */
  target();
  run(&target);
  const char *s = "target here";
}
`},
	{"a.cpp", 2, 2, `struct A { void target(); void caller(); };
void A::target() {}

void A::caller() {
  // see target
  target();
  run(&A::target);
  auto s = "target here";
}
`},
	{"a.cs", 2, 2, `class A {
  void target() {}

  void caller() {
    // see target
    target();
    Run(target);
    var s = "target here";
  }
}
`},
	{"a.rs", 1, 1, `fn target() {}

fn caller() {
    // see target
    target();
    run(target);
    let s = "target here";
}
`},
	{"a.rb", 1, 2, `def target
end

def caller
  # see target
  target()
  run(:target)
  s = "target here"
end
`},
	{"a.php", 2, 2, `<?php
function target() {}

function caller() {
  // see target
  target();
  run(target(...));
  $s = "target here";
}
`},
	{"a.lua", 1, 2, `local function target()
end

local function caller()
  -- see target
  target()
  run(target)
  local s = "target here"
end
`},
	{"a.sh", 1, 3, `target() {
  :
}

caller() {
  # see target
  target
  trap target EXIT
  echo "target here"
}
`},
}

func summary(refs []Reference) string {
	var parts []string
	for _, r := range refs {
		enc := "-"
		if r.Enclosing != nil {
			enc = r.Enclosing.Name
		}
		parts = append(parts, fmt.Sprintf("%s@%s", r.Kind, enc))
	}
	sort.Strings(parts)
	return strings.Join(parts, " ")
}

func TestLanguages(t *testing.T) {
	for _, s := range samples {
		t.Run(s.file, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, s.file), []byte(s.src), 0o644); err != nil {
				t.Fatal(err)
			}
			want := "argument@caller call@caller comment@caller string@caller"
			if s.file == "a.php" { // target(...) is first-class callable syntax: a call node
				want = "call@caller call@caller comment@caller string@caller"
			}

			if s.file == "a.cpp" { // the in-class declaration `void target();` is a reference
				want = "argument@caller call@caller comment@caller reference@- string@caller"
			}

			refs, err := FindByLines(dir, s.file, s.start, s.end)
			if err != nil {
				t.Fatal(err)
			}
			if got := summary(refs); got != want {
				for _, r := range refs {
					t.Logf("%+v", r)
				}
				t.Errorf("by lines:\n got  %s\n want %s", got, want)
			}

			refs, err = FindByName(dir, "target")
			if err != nil {
				t.Fatal(err)
			}
			defs := 0
			for _, r := range refs {
				if r.Kind == KindDefinition {
					defs++
				}
			}
			if defs == 0 {
				t.Errorf("by name: definition not reported: %s", summary(refs))
			}
		})
	}
}

func TestTextAndEnclosing(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		p := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("README.md", "# Doc\n\nCall `target` to start. Not retarget or target2.\n")
	write("node_modules/x/x.js", "target()\n")
	write("main.go", "package main\n\nfunc target() {}\n\nfunc main() {\n\tgo func() { x := compute(1 + target()) }()\n\tpkg.target()\n\tf := target\n}\n")

	refs, err := FindByName(dir, "target")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
	for _, r := range refs {
		got[string(r.Kind)+":"+r.File]++
		if r.File == "main.go" && r.Kind == KindCall {
			if r.Enclosing == nil || r.Enclosing.Name != "main" || r.Enclosing.StartLine != 5 || r.Enclosing.EndLine != 9 {
				t.Errorf("bad enclosing: %+v", r.Enclosing)
			}
		}
	}
	want := map[string]int{"text:README.md": 1, "definition:main.go": 1, "call:main.go": 2, "reference:main.go": 1}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("got %v want %v", got, want)
	}
}
