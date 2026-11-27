package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApproveNeedsATerminal(t *testing.T) {
	var out bytes.Buffer
	if ok, err := approve(strings.NewReader("y\n"), &out, false, "Install?"); ok || !errors.Is(err, errNoTerminal) {
		t.Errorf("a reader approved: %v, %v", ok, err)
	}
	null, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer null.Close()
	if ok, err := approve(null, &out, false, "Install?"); ok || !errors.Is(err, errNoTerminal) {
		t.Errorf("%s approved: %v, %v", os.DevNull, ok, err)
	}
	if ok, err := approve(strings.NewReader(""), &out, true, "Install?"); !ok || err != nil || out.Len() > 0 {
		t.Errorf("--yes: %v, %v, %q", ok, err, out.String())
	}
}

func TestAbsPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	wd, _ := os.Getwd()
	for in, want := range map[string]string{
		"~/p": filepath.Join(home, "p"), "~": home, "./p": filepath.Join(wd, "p"), "/a/../b": "/b",
	} {
		if got, err := absPath(in); err != nil || got != want {
			t.Errorf("absPath(%q) = %q, %v", in, got, err)
		}
	}
}
