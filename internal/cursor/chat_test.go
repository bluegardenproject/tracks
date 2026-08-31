package cursor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeAgent writes a stub executable that behaves like `agent` for the
// one subcommand CreateChat uses.
func fakeAgent(t *testing.T, script string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "agent")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCreateChatReturnsTheID(t *testing.T) {
	bin := fakeAgent(t, `echo 08ab06bf-8ec1-495f-90c9-b7523131b824`)
	got, err := CreateChat(context.Background(), bin)
	if err != nil {
		t.Fatal(err)
	}
	if got != "08ab06bf-8ec1-495f-90c9-b7523131b824" {
		t.Errorf("CreateChat = %q", got)
	}
}

// The real CLI prints certificate warnings before the id on this
// machine, and could print a hint after it. The id is found by
// matching a uuid on any line, first match wins — not by position.
func TestCreateChatFindsTheIDOnAnyLine(t *testing.T) {
	// Warnings before it today; a hint line after it is equally
	// plausible tomorrow, and a last-line heuristic would fail on that.
	bin := fakeAgent(t, `echo "ERROR: failed to copy trust settings"
echo 08ab06bf-8ec1-495f-90c9-b7523131b824
echo "Tip: run agent --help"`)
	got, err := CreateChat(context.Background(), bin)
	if err != nil {
		t.Fatal(err)
	}
	if got != "08ab06bf-8ec1-495f-90c9-b7523131b824" {
		t.Errorf("CreateChat = %q, want the id from the last line", got)
	}
}

// CreateChat must return rather than block. This does NOT pin the
// stdin behaviour that motivates the function: cmd.Stdin = nil is the
// zero value, so deleting that line changes nothing here, and the
// context deadline would kill a blocked child anyway. The stdin
// requirement is guarded by the comment at the assignment and by the
// out-of-band check against the real binary; this test claims only
// that a child producing no usable output is rejected promptly.
func TestCreateChatReturnsPromptlyOnEmptyOutput(t *testing.T) {
	bin := fakeAgent(t, `cat`) // reads to EOF; with /dev/null that is immediate
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		if _, err := CreateChat(ctx, bin); err == nil {
			t.Error("empty output should not be accepted as a chat id")
		}
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("CreateChat did not return")
	}
}

// A surviving grandchild holding the output pipe would block Output()
// past the context kill; WaitDelay bounds it.
func TestCreateChatSurvivesALingeringGrandchild(t *testing.T) {
	bin := fakeAgent(t, `sleep 60 & echo 08ab06bf-8ec1-495f-90c9-b7523131b824`)
	start := time.Now()
	done := make(chan struct{})
	go func() {
		defer close(done)
		if _, err := CreateChat(context.Background(), bin); err != nil {
			t.Logf("returned an error (acceptable): %v", err)
		}
	}()
	select {
	case <-done:
		if el := time.Since(start); el > 20*time.Second {
			t.Errorf("took %s — WaitDelay is not bounding the pipe wait", el)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("CreateChat blocked on a grandchild holding the output pipe")
	}
}

func TestCreateChatRejectsNonIDOutput(t *testing.T) {
	for name, script := range map[string]string{
		"empty":     `true`,
		"prose":     `echo "Please run agent login first"`,
		"truncated": `echo 08ab06bf-8ec1`,
		// Exactly 36 characters and no whitespace — accepted by a bare
		// length check, and would have been stored as the SessionID.
		"36-char prose": `echo "please run agent login to conti"`,
		"uppercase":     `echo 08AB06BF-8EC1-495F-90C9-B7523131B824`,
	} {
		t.Run(name, func(t *testing.T) {
			if got, err := CreateChat(context.Background(), fakeAgent(t, script)); err == nil {
				t.Errorf("accepted %q as a chat id", got)
			}
		})
	}
}

func TestCreateChatSurfacesTheCLIError(t *testing.T) {
	bin := fakeAgent(t, `echo "not authenticated" >&2; exit 1`)
	_, err := CreateChat(context.Background(), bin)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "not authenticated") {
		t.Errorf("error should carry the CLI's message, got: %v", err)
	}
}

func TestCreateChatNeedsABinary(t *testing.T) {
	if _, err := CreateChat(context.Background(), "  "); err == nil {
		t.Error("an unconfigured binary should fail before exec")
	}
}
