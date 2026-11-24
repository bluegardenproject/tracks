package train

import (
	"regexp"
	"strings"
	"testing"
)

func TestCheckPrefix(t *testing.T) {
	for p, ok := range map[string]bool{
		"reviewer": true, "watch-ci": true, "a": true, strings.Repeat("a", MaxPrefix): true,
		"": false, "1a": false, "Reviewer": false, "a_b": false, "a-": false, strings.Repeat("a", MaxPrefix+1): false,
		"track": false, "tracks": false,
	} {
		if err := CheckPrefix(p); (err == nil) != ok {
			t.Errorf("CheckPrefix(%q) = %v, want ok %v", p, err, ok)
		}
	}
}

func TestNewHandle(t *testing.T) {
	seen := map[string]bool{}
	for range 50 {
		h, err := NewHandle("reviewer", func(h string) bool { return seen[h] })
		if err != nil {
			t.Fatal(err)
		}
		if !regexp.MustCompile(`^reviewer-\d{6}$`).MatchString(h) || CheckHandle(h) != nil || seen[h] {
			t.Fatalf("handle %q", h)
		}
		seen[h] = true
	}
	long := strings.Repeat("a", MaxPrefix)
	if h, err := NewHandle(long, func(string) bool { return false }); err != nil || CheckHandle(h) != nil {
		t.Errorf("the longest prefix gave %q, %v", h, err)
	}
	if _, err := NewHandle("reviewer", func(string) bool { return true }); err == nil {
		t.Error("a handle came though every one was taken")
	}
	if _, err := NewHandle("Bad", func(string) bool { return false }); err == nil {
		t.Error("a handle came from a bad prefix")
	}
}

func TestResolve(t *testing.T) {
	handles := []string{"reviewer-527192", "watch-000001", "watch-000002", "reviewer-x"}
	for _, tc := range []struct{ name, want string }{
		{"reviewer-527192", "reviewer-527192"},
		{"reviewer", "reviewer-527192"},
		{"@reviewer-527192", "reviewer-527192"},
		{"@reviewer", "reviewer-527192"},
		{"watch-000002", "watch-000002"},
		{"watch", ""},
		{"detective", ""},
		{"review", ""},
	} {
		got, err := Resolve(tc.name, handles)
		if got != tc.want || (err == nil) != (tc.want != "") {
			t.Errorf("Resolve(%q) = %q, %v; want %q", tc.name, got, err, tc.want)
		}
	}
	if _, err := Resolve("watch", handles); err == nil || !strings.Contains(err.Error(), "watch-000001 or watch-000002") {
		t.Errorf("an ambiguous name: %v", err)
	}
}
