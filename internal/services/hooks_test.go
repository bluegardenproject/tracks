package services

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunHooksEmptyIsNoop(t *testing.T) {
	if err := RunHooks(context.Background(), nil, TemplateData{}, t.TempDir(), filepath.Join(t.TempDir(), "log")); err != nil {
		t.Errorf("empty hooks should be a no-op: %v", err)
	}
}

func TestRunHooksRunInOrder(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "hooks.log")
	cmds := []string{"printf 'a\\n'", "printf 'b\\n'", "printf 'c\\n'"}
	if err := RunHooks(context.Background(), cmds, TemplateData{}, dir, logPath); err != nil {
		t.Fatalf("RunHooks: %v", err)
	}
	got := readFile(t, logPath)
	if got != "a\nb\nc\n" {
		t.Errorf("hooks ran out of order or missing output: %q", got)
	}
}

func TestRunHooksTemplated(t *testing.T) {
	// Hooks are templated against the same data as cmd/env.
	dir := t.TempDir()
	logPath := filepath.Join(dir, "hooks.log")
	data := NewTemplateData("trk", dir, map[string]int{"web": 4100})
	cmds := []string{`printf 'port=%s\n' {{.Port "web"}}`}
	if err := RunHooks(context.Background(), cmds, data, dir, logPath); err != nil {
		t.Fatalf("RunHooks: %v", err)
	}
	if got := readFile(t, logPath); !strings.Contains(got, "port=4100") {
		t.Errorf("template not rendered in hook: %q", got)
	}
}

func TestRunHooksFailureAborts(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "hooks.log")
	// The middle hook fails; the third must never run.
	cmds := []string{"printf 'first\\n'", "false", "printf 'third\\n'"}
	err := RunHooks(context.Background(), cmds, TemplateData{}, dir, logPath)
	if err == nil {
		t.Fatal("expected error from failing hook")
	}
	got := readFile(t, logPath)
	if !strings.Contains(got, "first") {
		t.Errorf("first hook should have run: %q", got)
	}
	if strings.Contains(got, "third") {
		t.Errorf("third hook should not run after a failure: %q", got)
	}
}

func TestRunHooksBadTemplate(t *testing.T) {
	dir := t.TempDir()
	cmds := []string{`{{.Port "unknown"}}`}
	if err := RunHooks(context.Background(), cmds, NewTemplateData("t", dir, nil), dir, filepath.Join(dir, "log")); err == nil {
		t.Error("expected render error for unknown port")
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}
