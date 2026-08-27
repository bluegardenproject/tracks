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
// that shows a picker must put its answer in the params it returns.
//
// Checked for every picker, not just the model, because the failure is
// per-field: runDocReview bound the model picker and omitted Model
// from its literal, and a second picker is a second chance to do
// exactly that.
//
// This is not hypothetical. runDocReview declared `model`, bound it to
// modelField, and then omitted Model from its NewParams. The picker
// appeared, accepted a choice, and silently discarded it; the track ran
// the per-kind default and nothing said otherwise. `go vet` can't see
// it, because the variable *is* used — its address is taken.
func TestEveryFlowWithAPickerSendsIt(t *testing.T) {
	for _, pk := range []struct{ helper, field string }{
		{"modelField", "Model"},
		// providerFields, not providerField: the wrapper is what the
		// flows call, because the picker is omitted when only one
		// provider is installed.
		{"providerFields", "Provider"},
	} {
		t.Run(pk.helper, func(t *testing.T) { assertPickerReaches(t, pk.helper, pk.field) })
	}
}

func assertPickerReaches(t *testing.T, helper, field string) {
	t.Helper()
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
				if id, ok := x.Fun.(*ast.Ident); ok && id.Name == helper {
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
					if k, ok := kv.Key.(*ast.Ident); ok && k.Name == field {
						sendsModel = true
					}
				}
			}
			return true
		})

		// The helper itself builds the picker; it returns no params.
		if !showsPicker || fn.Name.Name == helper {
			continue
		}
		flowsChecked++
		if !sendsModel {
			t.Errorf("%s calls %s but its daemon.NewParams literal has no %s field — either the user's pick is silently discarded, or it is assigned outside the literal, which this check can't see",
				fn.Name.Name, helper, field)
		}
	}

	// Guard the guard: if modelField is renamed, the loop above matches
	// nothing and would pass while checking nothing at all.
	if flowsChecked < 3 {
		t.Fatalf("checked %d flows for %s, expected at least 3 (custom, review, doc review) — has it been renamed?", flowsChecked, helper)
	}
}

// The model list must be bound to the provider, not built from its
// value. With static Options the list freezes at form-construction
// time: switching Claude → Cursor leaves Claude's ids on offer, and an
// explicit pick then goes out as `agent --model claude-sonnet-4-6`.
//
// Checked against the source because the binding is huh's behaviour at
// runtime, not something modelField returns — a unit test on
// modelOptions passes either way, which is how the bug survived its
// first round of tests.
func TestModelFieldBindsTheProviderRatherThanCopyingIt(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "newtrack.go", nil, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	var found, usesOptionsFunc, usesStaticOptions bool
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "modelField" {
			continue
		}
		found = true
		ast.Inspect(fn, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			switch sel.Sel.Name {
			case "OptionsFunc":
				usesOptionsFunc = true
			case "Options":
				usesStaticOptions = true
			}
			return true
		})
	}
	if !found {
		t.Fatal("modelField not found — has it been renamed?")
	}
	if !usesOptionsFunc {
		t.Error("modelField does not use OptionsFunc; the list will not follow the provider picker")
	}
	if usesStaticOptions {
		t.Error("modelField uses static Options; the list is fixed at construction time")
	}
}
