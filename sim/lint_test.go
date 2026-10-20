package sim

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestDeterminismLint enforces AGENTS.md rule 4 on the non-test sources of the core: imports are a WHITELIST (no
// time, math, rand, os, reflect, runtime, sync, unsafe ... unless listed here), no floats (types or literals), no
// range over a map, no goroutines, channels or select. It covers every Go file of the core: the package sim in this
// directory and the JSON facade sim/wasm/wire; TestLintCoversAllCoreFiles fails when a new Go package appears
// that nobody lints.

// lintDirs are the directories of the core and the imports each may use. Everything else is a violation.
var lintDirs = map[string][]string{
	".":         {"errors", "fmt", "sort", "math/bits"},
	"wasm/wire": {"encoding/json", "fmt", "strconv", "github.com/OWNER/fireline/sim"},
}

// untestedDirs hold Go code that is not logic: wasm/ is the syscall/js glue (build tag js && wasm).
var untestedDirs = map[string]bool{"wasm": true}

func lintParse(t *testing.T, dir string) (*token.FileSet, map[string]*ast.File) {
	t.Helper()
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(fi os.FileInfo) bool { return !strings.HasSuffix(fi.Name(), "_test.go") }, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) != 1 {
		t.Fatalf("%s: want one package, got %d", dir, len(pkgs))
	}
	for _, p := range pkgs {
		return fset, p.Files
	}
	return nil, nil
}

// lintFiles returns one message per violation.
func lintFiles(t *testing.T, fset *token.FileSet, files map[string]*ast.File, allowed []string) []string {
	t.Helper()
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	var list []*ast.File
	for _, n := range names {
		list = append(list, files[n])
	}
	info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}}
	conf := types.Config{Importer: importer.ForCompiler(fset, "source", nil)}
	if _, err := conf.Check("lint", fset, list, info); err != nil {
		t.Fatal(err)
	}
	ok := map[string]bool{}
	for _, a := range allowed {
		ok[a] = true
	}
	var bad []string
	add := func(pos token.Pos, msg string) { bad = append(bad, fset.Position(pos).String()+": "+msg) }
	for _, f := range list {
		for _, imp := range f.Imports {
			if p := strings.Trim(imp.Path.Value, `"`); !ok[p] {
				add(imp.Pos(), "import "+p+" is not allowed in deterministic code")
			}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.Ident:
				if x.Name == "float32" || x.Name == "float64" {
					add(x.Pos(), "float type")
				}
			case *ast.BasicLit:
				if x.Kind == token.FLOAT || x.Kind == token.IMAG {
					add(x.Pos(), "float literal")
				}
			case *ast.RangeStmt:
				if tv, ok := info.Types[x.X]; ok {
					switch tv.Type.Underlying().(type) {
					case *types.Map:
						add(x.Pos(), "range over map")
					case *types.Chan:
						add(x.Pos(), "range over channel")
					}
				}
			case *ast.GoStmt:
				add(x.Pos(), "goroutine")
			case *ast.SelectStmt:
				add(x.Pos(), "select")
			case *ast.ChanType:
				add(x.Pos(), "channel")
			}
			return true
		})
	}
	return bad
}

func TestDeterminismLint(t *testing.T) {
	for dir, allowed := range lintDirs {
		fset, files := lintParse(t, dir)
		if len(files) == 0 {
			t.Fatalf("%s: no files to lint", dir)
		}
		for _, v := range lintFiles(t, fset, files, allowed) {
			t.Error(v)
		}
	}
}

// Every non-test Go file under sim/ belongs to a linted directory (or to the glue allow-list).
func TestLintCoversAllCoreFiles(t *testing.T) {
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "testdata" || d.Name() == "dist" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		dir := filepath.ToSlash(filepath.Dir(path))
		if _, linted := lintDirs[dir]; !linted && !untestedDirs[dir] {
			t.Errorf("%s: directory %q is neither linted (lintDirs) nor listed as glue (untestedDirs)", path, dir)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := lintDirs["."]; !ok {
		t.Fatal("the package sim itself must be linted")
	}
}

// The lint really detects each class of violation (a lint that never fires protects nothing).
func TestLintDetectsViolations(t *testing.T) {
	cases := map[string]string{
		"import time":       "package p\nimport \"time\"\nvar _ = time.Now\n",
		"import math/rand":  "package p\nimport \"math/rand\"\nvar _ = rand.Int\n",
		"import os":         "package p\nimport \"os\"\nvar _ = os.Args\n",
		"import reflect":    "package p\nimport \"reflect\"\nvar _ = reflect.TypeOf\n",
		"float type":        "package p\nvar x float64\n",
		"float literal":     "package p\nvar x = 1.5\n",
		"range over map":    "package p\nfunc f(m map[int]int) { for range m {} }\n",
		"range over map #2": "package p\ntype M map[string]int\nfunc f(m M) { for k, v := range m { _, _ = k, v } }\n",
		"goroutine":         "package p\nfunc f() { go f() }\n",
		"select":            "package p\nfunc f(c chan int) { select { case <-c: } }\n",
		"channel":           "package p\nvar c chan int\n",
	}
	for name, src := range cases {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, "p.go", src, 0)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if bad := lintFiles(t, fset, map[string]*ast.File{"p.go": f}, nil); len(bad) == 0 {
			t.Errorf("%s: the lint did not fire", name)
		}
	}
	// and it stays quiet on clean code
	fset := token.NewFileSet()
	f, _ := parser.ParseFile(fset, "p.go", "package p\nfunc f(s []int) (n int) { for _, v := range s { n += v }; return }\n", 0)
	if bad := lintFiles(t, fset, map[string]*ast.File{"p.go": f}, nil); len(bad) != 0 {
		t.Errorf("false positive: %v", bad)
	}
}
