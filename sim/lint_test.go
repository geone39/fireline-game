package sim

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"strings"
	"testing"
)

// TestDeterminismLint enforces AGENTS.md rule 4 on non-test sources of this package:
// no time / math / rand imports, no floats, no iteration over maps.
func TestDeterminismLint(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool { return !strings.HasSuffix(fi.Name(), "_test.go") }, 0)
	if err != nil {
		t.Fatal(err)
	}
	pkg := pkgs["sim"]
	if pkg == nil {
		t.Fatal("package sim not found")
	}
	var files []*ast.File
	for _, f := range pkg.Files {
		files = append(files, f)
	}
	info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}}
	conf := types.Config{Importer: importer.ForCompiler(fset, "source", nil)}
	if _, err := conf.Check("sim", fset, files, info); err != nil {
		t.Fatal(err)
	}
	banned := map[string]bool{"time": true, "math/rand": true, "math/rand/v2": true, "crypto/rand": true, "math": true}
	for name, f := range pkg.Files {
		for _, imp := range f.Imports {
			if p := strings.Trim(imp.Path.Value, `"`); banned[p] {
				t.Errorf("%s: forbidden import %s", name, p)
			}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.Ident:
				if x.Name == "float32" || x.Name == "float64" {
					t.Errorf("%s: float type", fset.Position(x.Pos()))
				}
			case *ast.BasicLit:
				if x.Kind == token.FLOAT {
					t.Errorf("%s: float literal", fset.Position(x.Pos()))
				}
			case *ast.RangeStmt:
				if tv, ok := info.Types[x.X]; ok {
					if _, isMap := tv.Type.Underlying().(*types.Map); isMap {
						t.Errorf("%s: range over map", fset.Position(x.Pos()))
					}
				}
			}
			return true
		})
	}
}
