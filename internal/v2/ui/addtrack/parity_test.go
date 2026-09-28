package addtrack

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
)

const v1Form = "../../../tui/newtrack/"

// v2Names swaps the helper names v2 renames to run beside v1.
var v2Names = strings.NewReplacer("tracks-reviewer", "tracks-v2-reviewer", "tracks-docs-reviewer", "tracks-v2-docs-reviewer")

// The form copies v1's texts; these fail when one side changes alone.
func TestTextsMatchV1(t *testing.T) {
	prompts := v1Strings(t, v1Form+"templates.go", "templatePrompts")
	about := v1Strings(t, v1Form+"templates.go", "templateDescriptions")
	names := map[Kind]string{Work: "TemplateCustom", Ask: "TemplateAsk", Plan: "TemplatePlan", Review: "TemplateReview", Doc: "TemplateDocReview"}
	for k, name := range names {
		if want := v2Names.Replace(prompts[name]); kinds[k].prompt != want {
			t.Errorf("%s prompt differs from v1's %s:\n%q\n%q", kinds[k].label, name, kinds[k].prompt, want)
		}
		if kinds[k].about != about[name] {
			t.Errorf("%s description differs from v1's %s:\n%q\n%q", kinds[k].label, name, kinds[k].about, about[name])
		}
	}
	options := v1Options(t, v1Form+"newtrack.go", "docSectionOpinion", "docSectionClaimCheck")
	for i, want := range options {
		if sections[i] != want {
			t.Errorf("section %d is %q, v1 has %q", i, sections[i], want)
		}
	}
}

// v1Strings reads the map literal var name in file: its keys' names to
// their string values.
func v1Strings(t *testing.T, file, name string) map[string]string {
	t.Helper()
	out := map[string]string{}
	ast.Inspect(parse(t, file), func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok || len(spec.Names) != 1 || spec.Names[0].Name != name {
			return true
		}
		for _, elt := range spec.Values[0].(*ast.CompositeLit).Elts {
			kv := elt.(*ast.KeyValueExpr)
			out[kv.Key.(*ast.Ident).Name] = stringOf(t, kv.Value)
		}
		return false
	})
	if len(out) == 0 {
		t.Fatalf("no %s in %s", name, file)
	}
	return out
}

// v1Options reads the labels of the huh options in file whose values
// are values, in that order.
func v1Options(t *testing.T, file string, values ...string) []string {
	t.Helper()
	labels := map[string]string{}
	ast.Inspect(parse(t, file), func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) != 2 {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "NewOption" {
			if value, ok := call.Args[1].(*ast.Ident); ok {
				if _, ok := call.Args[0].(*ast.BasicLit); ok {
					labels[value.Name] = stringOf(t, call.Args[0])
				}
			}
		}
		return true
	})
	out := make([]string, len(values))
	for i, v := range values {
		if out[i] = labels[v]; out[i] == "" {
			t.Fatalf("no option %s in %s", v, file)
		}
	}
	return out
}

func parse(t *testing.T, file string) *ast.File {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// stringOf evaluates a string literal or a sum of them.
func stringOf(t *testing.T, e ast.Expr) string {
	t.Helper()
	switch e := e.(type) {
	case *ast.BasicLit:
		s, err := strconv.Unquote(e.Value)
		if err != nil {
			t.Fatal(err)
		}
		return s
	case *ast.BinaryExpr:
		if e.Op == token.ADD {
			return stringOf(t, e.X) + stringOf(t, e.Y)
		}
	}
	t.Fatalf("not a string: %T", e)
	return ""
}
