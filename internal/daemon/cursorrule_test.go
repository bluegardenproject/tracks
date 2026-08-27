package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The rule loads in EVERY Cursor session the user runs, including
// their ordinary work. If it does not gate itself on TRACKS_ID it is
// telling people about worktrees they are not in.
func TestCursorRuleGatesItselfOnTracksID(t *testing.T) {
	head := cursorRuleTemplate
	if i := strings.Index(head, "# tracks worktrees"); i > 0 {
		head = head[i:]
	}
	first := strings.SplitN(strings.TrimSpace(head), "\n\n", 3)
	body := strings.Join(first, "\n\n")
	if !strings.Contains(body[:min(400, len(body))], "TRACKS_ID") {
		t.Error("the gate is not in the opening lines; a reader skimming would not see it")
	}
	if !strings.Contains(cursorRuleTemplate, "ignore everything below") {
		t.Error("no instruction to ignore the rule outside a track")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// alwaysApply is what makes Cursor load it without being asked; the
// marker is what lets tracks update it later.
func TestCursorRuleFrontmatter(t *testing.T) {
	for _, want := range []string{`x-tracks-managed: "1"`, "alwaysApply: true", "description:"} {
		if !strings.Contains(cursorRuleTemplate, want) {
			t.Errorf("frontmatter is missing %q", want)
		}
	}
	if !strings.HasPrefix(cursorRuleTemplate, "---\n") {
		t.Error("frontmatter must open the file or Cursor will not parse it")
	}
}

// Its cost is paid on every unrelated Cursor session, so it has to
// stay a page, not a manual. The spawn prompt carries the detail.
func TestCursorRuleStaysShort(t *testing.T) {
	if n := len(cursorRuleTemplate); n > 2500 {
		t.Errorf("rule is %d bytes; it loads in every Cursor session the user runs, so keep it under ~2500", n)
	}
}

// It must carry no per-track values: one static file serves every
// track, with the specifics read from the environment.
func TestCursorRuleHasNoPerTrackValues(t *testing.T) {
	for _, leak := range []string{"/Users/", "/tmp/", "20260", "socket-"} {
		if strings.Contains(cursorRuleTemplate, leak) {
			t.Errorf("rule contains %q — it should be static and read specifics from the env", leak)
		}
	}
}

// A Claude-only user should not get a ~/.cursor directory.
func TestCursorRuleSkippedWhenCursorIsAbsent(t *testing.T) {
	home := t.TempDir()
	cfg := testConfig(t)
	cfg.Cursor.Binary = filepath.Join(t.TempDir(), "definitely-not-installed")
	srv := NewServer(cfg, nil, "test")

	if err := srv.installCursorRule(home); err != nil {
		t.Fatalf("a missing binary should not be an error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".cursor")); !os.IsNotExist(err) {
		t.Error("created ~/.cursor for a user without Cursor installed")
	}
}

// And it must actually install when Cursor is there.
func TestCursorRuleInstalledWhenCursorIsPresent(t *testing.T) {
	home := t.TempDir()
	fake := filepath.Join(t.TempDir(), "agent")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := testConfig(t)
	cfg.Cursor.Binary = fake
	srv := NewServer(cfg, nil, "test")

	if err := srv.installCursorRule(home); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(home, ".cursor", "rules", "tracks.mdc"))
	if err != nil {
		t.Fatalf("rule not installed: %v", err)
	}
	if string(got) != cursorRuleTemplate {
		t.Error("installed content differs from the template")
	}
}

// ~/.cursor/rules is a directory users populate themselves — there are
// hand-written rules in it on the author's machine — so the ownership
// check matters more here than anywhere else tracks writes.
func TestCursorRuleWillNotClobberAUserRule(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".cursor", "rules", "tracks.mdc")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	mine := "---\ndescription: my own rule about tracks\nalwaysApply: true\n---\nmine"
	if err := os.WriteFile(path, []byte(mine), 0o644); err != nil {
		t.Fatal(err)
	}

	fake := filepath.Join(t.TempDir(), "agent")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := testConfig(t)
	cfg.Cursor.Binary = fake
	srv := NewServer(cfg, nil, "test")

	if err := srv.installCursorRule(home); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != mine {
		t.Error("overwrote a rule the user wrote")
	}
}
