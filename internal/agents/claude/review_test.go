package claude

import (
	"slices"
	"testing"
)

func TestReviewCommandOnlyReads(t *testing.T) {
	c := ReviewCommand("/bin/claude", "opus")
	i := slices.Index(c.Args, "--tools")
	if i < 0 || c.Args[i+1] != "Read,Grep,Glob" {
		t.Errorf("args %v: the reviewer's tools must be Read, Grep and Glob only", c.Args)
	}
	for _, flag := range []string{"-p", "--restricted", "--strict-mcp-config"} {
		if !slices.Contains(c.Args, flag) {
			t.Errorf("args %v lack %s", c.Args, flag)
		}
	}
	for _, flag := range []string{"--dangerously-skip-permissions", "bypassPermissions", "acceptEdits", "auto", "--allowedTools"} {
		if slices.Contains(c.Args, flag) {
			t.Errorf("args %v carry %s", c.Args, flag)
		}
	}
	if !slices.Equal(c.Args[len(c.Args)-2:], []string{"--model", "opus"}) || c.Binary != "/bin/claude" {
		t.Errorf("command %+v", c)
	}
	if d := ReviewCommand("", ""); d.Binary != "claude" || slices.Contains(d.Args, "--model") {
		t.Error("no binary or model should leave Claude's defaults")
	}
}
