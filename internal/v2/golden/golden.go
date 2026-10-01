// Package golden compares test output with files kept in testdata.
// Run the tests with -update to rewrite the files, then review the
// diff: a golden file changes only on purpose.
package golden

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the golden files")

// Check fails t when got differs from the file at path, or rewrites the
// file with -update.
func Check(t *testing.T, path, got string) {
	t.Helper()
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run the tests with -update to write it)", err)
	}
	if got != string(want) {
		t.Errorf("%s differs; rerun with -update and review the diff\n got: %s\nwant: %s", path, got, want)
	}
}
