package commentstyle

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bluegardenproject/tracks/internal/golden"
)

var now = time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)

func at(daysAgo int) time.Time { return now.Add(-time.Duration(daysAgo) * 24 * time.Hour) }

func TestKeep(t *testing.T) {
	me, bot, alice := ghUser{Login: "me", Type: "User"}, ghUser{Login: "copilot-pull-request-reviewer", Type: "Bot"}, ghUser{Login: "alice", Type: "User"}
	list := []ghComment{
		{ID: 1, User: bot, Body: "Consider a guard here.", CreatedAt: at(3)},
		{ID: 2, InReplyTo: 1, User: me, Body: "Added the guard.", CreatedAt: at(3)},
		{ID: 3, User: alice, Body: "Why not a map?", CreatedAt: at(2)},
		{ID: 4, InReplyTo: 3, User: me, Body: "Order matters here, so a slice.", CreatedAt: at(2)},
		{ID: 5, InReplyTo: 3, User: me, Body: "Switched to a map.\n\n" + Marker, CreatedAt: at(1)},
		{ID: 6, User: me, Body: "Too old to count.", CreatedAt: at(90)},
		{ID: 7, User: me, Body: "  ", CreatedAt: at(1)},
	}
	kept, s := keep(list, "me", now.Add(-Window))
	if len(kept) != 1 || kept[0].ID != 4 {
		t.Errorf("kept %+v; want only the reply to a person, without the marker", kept)
	}
	if s.tracks != 1 || s.bots != 1 {
		t.Errorf("left out %+v; want one by Tracks, one in a bot thread", s)
	}
}

// fakeAPI answers paths from answers, recording what was asked.
type fakeAPI struct {
	answers map[string]string
	asked   []string
}

func (f *fakeAPI) Get(_ context.Context, path string, _ ...string) ([]byte, error) {
	f.asked = append(f.asked, path)
	a, ok := f.answers[path]
	if !ok {
		return nil, fmt.Errorf("no answer for %s", path)
	}
	return []byte(a), nil
}

func newFakeAPI() *fakeAPI {
	return &fakeAPI{answers: map[string]string{
		"user":          `{"login":"me"}`,
		"search/issues": `{"total_count":1,"items":[{"number":23,"repository_url":"https://api.github.com/repos/acme/api"}]}`,
		"repos/acme/api/pulls/23/comments": `[{"id":1,"user":{"login":"alice","type":"User"},"body":"Rename this?","created_at":"2026-10-10T10:00:00Z"},
			{"id":2,"in_reply_to_id":1,"user":{"login":"me","type":"User"},"body":"Good call, renamed.","created_at":"2026-10-10T11:00:00Z"}]`,
		"repos/acme/api/issues/23/comments": `[{"id":9,"user":{"login":"me","type":"User"},"body":"Ready for another look.","created_at":"2026-10-11T09:00:00Z"}]`,
	}}
}

func TestLearn(t *testing.T) {
	dir := t.TempDir()
	engine := filepath.Join(dir, "claude")
	stdin := filepath.Join(dir, "stdin")
	script := "#!/bin/sh\ncat > " + stdin + "\necho \"$*\" > " + stdin + ".args\nprintf '## Tone\\nShort and friendly.\\n'\n"
	if err := os.WriteFile(engine, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config", FileName)
	var log []string
	l := Learner{API: newFakeAPI(), Engine: Engine{ID: "claude", Name: "Claude Code", Program: engine, Model: "opus"}, Now: func() time.Time { return now }}
	st, err := l.Learn(context.Background(), path, func(line Line) { log = append(log, line.String()) })
	if err != nil {
		t.Fatalf("Learn: %v\n%s", err, strings.Join(log, "\n"))
	}
	if st != (Status{Learned: true, At: now, Comments: 2}) {
		t.Errorf("Learn = %+v", st)
	}
	prompt, _ := os.ReadFile(stdin)
	for _, want := range []string{"Good call, renamed.", "Ready for another look.", "acme/api#23 (review comment"} {
		if !strings.Contains(string(prompt), want) {
			t.Errorf("the prompt lacks %q", want)
		}
	}
	if strings.Contains(string(prompt), "Rename this?") {
		t.Error("someone else's comment went into the prompt")
	}
	if args, _ := os.ReadFile(stdin + ".args"); strings.TrimSpace(string(args)) != "-p --model opus" {
		t.Errorf("the engine ran with %q", args)
	}
	got, err := ReadStatus(path)
	if err != nil || got != st {
		t.Errorf("ReadStatus = %+v, %v; want what Learn returned", got, err)
	}
	if b, _ := os.ReadFile(path); !strings.Contains(string(b), "Short and friendly.") {
		t.Errorf("the guide reads %q", b)
	}
	if last := log[len(log)-1]; !strings.HasPrefix(last, "✓ saved ") {
		t.Errorf("the log ends with %q", last)
	}
}

func TestLearnWithoutComments(t *testing.T) {
	api := newFakeAPI()
	api.answers["search/issues"] = `{"total_count":0,"items":[]}`
	l := Learner{API: api, Engine: Engine{Program: "/nonexistent"}, Now: func() time.Time { return now }}
	if _, err := l.Learn(context.Background(), filepath.Join(t.TempDir(), FileName), func(Line) {}); err == nil {
		t.Error("learned from nothing")
	}
}

func TestReadStatus(t *testing.T) {
	dir := t.TempDir()
	if st, err := ReadStatus(filepath.Join(dir, "missing.md")); err != nil || st.Learned {
		t.Errorf("a missing guide: %+v, %v", st, err)
	}
	hand := filepath.Join(dir, "hand.md")
	if err := os.WriteFile(hand, []byte("# My style\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if st, err := ReadStatus(hand); err != nil || !st.Learned || !st.At.IsZero() {
		t.Errorf("a guide written by hand: %+v, %v; want learned, with no date", st, err)
	}
}

func TestParseLine(t *testing.T) {
	for _, l := range []Line{{Request, "GET user"}, {Answer, "you're me"}, {Note, "kept 2"}, {Fail, "boom"}, {Done, "saved"}} {
		if got := ParseLine(l.String()); got != l {
			t.Errorf("ParseLine(%q) = %+v", l.String(), got)
		}
	}
	if got := ParseLine("plain"); got != (Line{Note, "plain"}) {
		t.Errorf("ParseLine(plain) = %+v", got)
	}
}

func TestPromptMatchesGolden(t *testing.T) {
	golden.Check(t, "testdata/prompt.golden", Prompt([]Comment{
		{PR: "acme/api#23", Kind: "review", Body: "Good call, renamed.", At: at(4)},
	}))
}
