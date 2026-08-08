package dlog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrintfBeforeInitDoesNotPanic(t *testing.T) {
	Close()
	Printf("no sink yet: %d", 1)
	if Path() != "" {
		t.Fatalf("Path() = %q, want empty before Init", Path())
	}
}

func TestInitWritesToFile(t *testing.T) {
	dir := t.TempDir()
	p, err := Init(dir)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer Close()

	if want := filepath.Join(dir, FileName); p != want {
		t.Fatalf("Init returned %q, want %q", p, want)
	}
	if Path() != p {
		t.Fatalf("Path() = %q, want %q", Path(), p)
	}

	Printf("hello %s", "world")
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.Contains(string(data), "hello world") {
		t.Fatalf("log does not contain the message: %q", string(data))
	}
}

// A second daemon run must not truncate the previous run's log — the crash
// being diagnosed is usually in the run before the current one.
func TestInitAppendsAcrossRuns(t *testing.T) {
	dir := t.TempDir()
	if _, err := Init(dir); err != nil {
		t.Fatalf("Init: %v", err)
	}
	Printf("first run")
	Close()

	p, err := Init(dir)
	if err != nil {
		t.Fatalf("Init again: %v", err)
	}
	defer Close()
	Printf("second run")

	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, "first run") || !strings.Contains(got, "second run") {
		t.Fatalf("log lost a run: %q", got)
	}
}

func TestCloseRevertsToStderr(t *testing.T) {
	dir := t.TempDir()
	if _, err := Init(dir); err != nil {
		t.Fatalf("Init: %v", err)
	}
	Close()
	if Path() != "" {
		t.Fatalf("Path() = %q after Close, want empty", Path())
	}
	Printf("still fine")
}
