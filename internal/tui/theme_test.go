package tui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// Forms need a TTY, so the rule that keeps them consistent is checked
// against the source: under internal/tui, every form runs through
// RunForm. A form calling .Run() itself silently loses the tracks theme
// and Esc-to-back. The only other .Run() allowed is a bubbletea program.
func TestFormsRunThroughRunForm(t *testing.T) {
	var runForms int
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") ||
			strings.HasSuffix(path, "_test.go") || path == "theme.go" {
			return err
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			switch {
			case sel.Sel.Name == "RunForm":
				runForms++
			case sel.Sel.Name == "Run" && len(call.Args) == 0 && !isTeaProgram(sel.X):
				t.Errorf("%s: .Run() called directly — run forms with tui.RunForm", path)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if runForms < 20 {
		t.Fatalf("found %d tui.RunForm calls, expected at least 20 — has it been renamed?", runForms)
	}
}

func isTeaProgram(x ast.Expr) bool {
	call, ok := x.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "tea" && sel.Sel.Name == "NewProgram"
}
