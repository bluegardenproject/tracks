package newtrack

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"strings"
	"testing"
)

// Every creation flow builds its own daemon.NewParams, and the forms
// can't be driven headlessly — they need a TTY. So the one invariant
// that actually broke is checked against the source instead: a flow
// that shows the model picker must put the answer in the params it
// returns.
//
// This is not hypothetical. runDocReview declared `model`, bound it to
// modelField, and then omitted Model from its NewParams. The picker
// appeared, accepted a choice, and silently discarded it; the track ran
// the per-kind default and nothing said otherwise. `go vet` can't see
// it, because the variable *is* used — its address is taken.
func TestEveryFlowWithAModelPickerSendsIt(t *testing.T) {
	// The whole package, not just newtrack.go: a fourth flow added in a
	// new file would otherwise be invisible here, and the floor below
	// would still see three.
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parse package: %v", err)
	}

	var decls []ast.Decl
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			decls = append(decls, file.Decls...)
		}
	}

	var flowsChecked int
	for _, decl := range decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}

		var showsPicker, sendsModel bool
		ast.Inspect(fn, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.CallExpr:
				if id, ok := x.Fun.(*ast.Ident); ok && id.Name == "modelField" {
					showsPicker = true
				}
			case *ast.CompositeLit:
				sel, ok := x.Type.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "NewParams" {
					return true
				}
				for _, el := range x.Elts {
					kv, ok := el.(*ast.KeyValueExpr)
					if !ok {
						continue
					}
					if k, ok := kv.Key.(*ast.Ident); ok && k.Name == "Model" {
						sendsModel = true
					}
				}
			}
			return true
		})

		// modelField itself builds the picker; it returns no params.
		if !showsPicker || fn.Name.Name == "modelField" {
			continue
		}
		flowsChecked++
		if !sendsModel {
			t.Errorf("%s shows the model picker but its daemon.NewParams literal has no Model field — either the user's pick is silently discarded, or it is assigned outside the literal, which this check can't see",
				fn.Name.Name)
		}
	}

	// Guard the guard: if modelField is renamed, the loop above matches
	// nothing and would pass while checking nothing at all.
	if flowsChecked < 3 {
		t.Fatalf("checked %d flows, expected at least 3 (custom, review, doc review) — has modelField been renamed?", flowsChecked)
	}
}
