package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	userPrompt = `{"type":"user","message":{"role":"user","content":"%s"}}`
	toolResult = `{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"ok"}]}}`
	meta       = `{"type":"user","isMeta":true,"message":{"role":"user","content":"<command-name>/model</command-name>"}}`
	said       = `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"%s"}]}}`
	readFile   = `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Read","input":{"file_path":"/x"}}]}}`
	planned    = `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t2","name":"ExitPlanMode","input":{"plan":"%s"}}]}}`
)

func line(format string, args ...any) string {
	if len(args) == 0 {
		return format
	}
	return strings.Replace(format, "%s", args[0].(string), 1)
}

func TestLastReply(t *testing.T) {
	for name, c := range map[string]struct {
		lines []string
		want  string
	}{
		"the last answer": {[]string{
			line(userPrompt, "Why is it slow?"), line(said, "Looking."), readFile, toolResult, line(said, "The cache is cold."),
		}, "Looking.\n\nThe cache is cold."},
		"a plan": {[]string{
			line(userPrompt, "Plan it"), readFile, toolResult, line(planned, "1. Warm the cache"), toolResult, line(said, "Waiting for approval."),
		}, "1. Warm the cache"},
		"an answer after the plan": {[]string{
			line(userPrompt, "Plan it"), line(planned, "1. Warm the cache"), line(userPrompt, "What about the TTL?"), line(said, "Keep it at 5 minutes."),
		}, "Keep it at 5 minutes."},
		"a turn without text": {[]string{
			line(userPrompt, "Why?"), line(said, "Because."), line(userPrompt, "Read x"), readFile, toolResult, meta,
		}, "Because."},
		"nothing": {[]string{line(userPrompt, "Hi"), "not json"}, ""},
	} {
		if got := lastReply(strings.NewReader(strings.Join(c.lines, "\n"))); got != c.want {
			t.Errorf("%s: %q, want %q", name, got, c.want)
		}
	}
}

func TestLastReplyFindsTheSession(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	project := filepath.Join(dir, "projects", "-src-api")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	transcript := strings.Join([]string{line(userPrompt, "Plan it"), line(planned, "1. Warm the cache")}, "\n")
	if err := os.WriteFile(filepath.Join(project, "s1.jsonl"), []byte(transcript), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := LastReply("s1"); got != "1. Warm the cache" {
		t.Errorf("LastReply = %q", got)
	}
	if got := LastReply("s2"); got != "" {
		t.Errorf("another session's reply: %q", got)
	}
}

func TestCarriedIsCut(t *testing.T) {
	long := strings.Repeat("é", maxCarried)
	got := carried(long)
	if !strings.HasSuffix(got, "\n\n[cut at 20 KB]") || len(got) > maxCarried+len("\n\n[cut at 20 KB]") {
		t.Fatalf("cut to %d bytes", len(got))
	}
	if body := strings.TrimSuffix(got, "\n\n[cut at 20 KB]"); strings.ContainsRune(body, '\uFFFD') || len(body)%2 != 0 {
		t.Error("cut inside a character")
	}
	if carried("short") != "short" {
		t.Error("a short reply was changed")
	}
}

func TestStarted(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	project := filepath.Join(dir, "projects", "-src-api")
	if err := os.MkdirAll(project, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "s1.jsonl"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !Started("s1") || Started("s2") {
		t.Errorf("Started(s1) = %v, Started(s2) = %v; want only the session with a transcript", Started("s1"), Started("s2"))
	}
}
