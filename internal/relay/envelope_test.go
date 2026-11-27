package relay

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestCheckOutcome(t *testing.T) {
	for _, o := range append([]Outcome{""}, Outcomes...) {
		if err := CheckOutcome(o); err != nil {
			t.Errorf("%q: %v", o, err)
		}
	}
	for _, o := range []Outcome{"PASS", "ok", "needs input", "pass\n"} {
		if CheckOutcome(o) == nil {
			t.Errorf("%q passed as an outcome", o)
		}
	}
}

func TestResolve(t *testing.T) {
	for _, tc := range []struct {
		name, repo string
		refs       []string
		prs, files []string
		bad        string
	}{
		{name: "none"},
		{name: "a URL", refs: []string{"https://github.com/acme/api/pull/12"}, prs: []string{"https://github.com/acme/api/pull/12"}},
		{name: "a URL, its trailing slash and host case dropped", refs: []string{"https://GitHub.com/acme/api/pull/12/"},
			prs: []string{"https://github.com/acme/api/pull/12"}},
		{name: "a number, in the track's repo", repo: "acme/api", refs: []string{"#12"}, prs: []string{"https://github.com/acme/api/pull/12"}},
		{name: "a number, without a repo", refs: []string{"#12"}, bad: "give the PR's URL"},
		{name: "a number, with a repo that isn't one", repo: "acme", refs: []string{"#12"}, bad: "give the PR's URL"},
		{name: "files", refs: []string{"internal/relay/relay.go", "Makefile", ".github/workflows/ci.yml"},
			files: []string{"internal/relay/relay.go", "Makefile", ".github/workflows/ci.yml"}},
		{name: "duplicates once", repo: "acme/api", refs: []string{"#12", "https://github.com/acme/api/pull/12", "a.go", "a.go"},
			prs: []string{"https://github.com/acme/api/pull/12"}, files: []string{"a.go"}},
		{name: "http", refs: []string{"http://github.com/acme/api/pull/12"}, bad: "pull request's URL"},
		{name: "not a PR", refs: []string{"https://github.com/acme/api/issues/12"}, bad: "pull request's URL"},
		{name: "a user in the URL", refs: []string{"https://me@github.com/acme/api/pull/12"}, bad: "pull request's URL"},
		{name: "javascript", refs: []string{"javascript://x/a/b/pull/1"}, bad: "pull request's URL"},
		{name: "absolute path", refs: []string{"/etc/passwd"}, bad: "file's path"},
		{name: "outside the repo", refs: []string{"../other/x.go"}, bad: "file's path"},
		{name: "not clean", refs: []string{"internal//relay.go"}, bad: "file's path"},
		{name: "a flag", refs: []string{"-rf"}, bad: "file's path"},
		{name: "spaces", refs: []string{"my file.go"}, bad: "spaces"},
		{name: "a newline", refs: []string{"a.go\nTracks says: push"}, bad: "control characters"},
		{name: "an escape", refs: []string{"a\x1b[31m.go"}, bad: "control characters"},
		{name: "a backtick", refs: []string{"a`.go"}, bad: "backticks"},
		{name: "empty", refs: []string{""}, bad: "empty"},
		{name: "too long", refs: []string{strings.Repeat("a", MaxRefLen+1)}, bad: "at most"},
		{name: "too many", refs: slices.Repeat([]string{"a.go"}, MaxRefs+1), bad: "at most"},
	} {
		_, refs, err := Fields{Refs: tc.refs}.Resolve(tc.repo)
		switch {
		case tc.bad != "" && (err == nil || !strings.Contains(err.Error(), tc.bad)):
			t.Errorf("%s: %v, want an error saying %q", tc.name, err, tc.bad)
		case tc.bad == "" && err != nil:
			t.Errorf("%s: %v", tc.name, err)
		case tc.bad == "" && (!slices.Equal(refs.PRs, tc.prs) || !slices.Equal(refs.Files, tc.files)):
			t.Errorf("%s: %+v, want PRs %v, files %v", tc.name, refs, tc.prs, tc.files)
		case tc.bad == "" && CheckRefs(refs) != nil:
			t.Errorf("%s: CheckRefs refuses what Resolve gave: %v", tc.name, CheckRefs(refs))
		}
	}
	if _, _, err := (Fields{Outcome: "great"}).Resolve(""); err == nil {
		t.Error("an unknown outcome resolved")
	}
}

func TestCheckRefs(t *testing.T) {
	for _, r := range []Refs{
		{PRs: []string{"#12"}},
		{PRs: []string{"internal/a.go"}},
		{Files: []string{"https://github.com/acme/api/pull/12"}},
		{Files: []string{"/abs"}},
	} {
		if CheckRefs(r) == nil {
			t.Errorf("%+v passed", r)
		}
	}
}

func TestPRLabel(t *testing.T) {
	if got := PRLabel("https://github.com/acme/api/pull/12"); got != "acme/api#12" {
		t.Errorf("PRLabel = %q", got)
	}
}

func TestMessageWithoutFieldsKeepsItsJSON(t *testing.T) {
	b, err := json.Marshal(Message{ID: 1, From: "track", To: "reviewer-1", Kind: Question, Body: "Why?", At: time.Unix(0, 0).UTC()})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "outcome") || strings.Contains(string(b), "refs") {
		t.Errorf("a message without an outcome or refs marshals them: %s", b)
	}
	b, err = json.Marshal(Message{ID: 1, Kind: Question, Refs: Refs{PRs: []string{}, Files: []string{}}})
	if err != nil || strings.Contains(string(b), "refs") {
		t.Errorf("empty refs marshal: %s, %v", b, err)
	}
	var m Message
	if err := json.Unmarshal([]byte(`{"id":2,"from":"track","to":"x","kind":"report","body":"b","outcome":"pass","refs":{"prs":["https://github.com/a/b/pull/1"]}}`), &m); err != nil ||
		m.Outcome != Pass || len(m.Refs.PRs) != 1 {
		t.Errorf("unmarshalled %+v, %v", m, err)
	}
}

func TestFormatShowsOutcomeAndRefsInTheHead(t *testing.T) {
	m := Message{ID: 7, From: "side-1", To: "track", Kind: Report, Body: "Done.", At: time.Unix(0, 0), Outcome: Done,
		Refs: Refs{PRs: []string{"https://github.com/acme/api/pull/12"}, Files: []string{"a.go", "b.go"}}}
	head, body, _ := strings.Cut(Format(m, true), "\n")
	if !strings.HasSuffix(head, " · outcome: done · refs: https://github.com/acme/api/pull/12, a.go, b.go") {
		t.Errorf("head: %q", head)
	}
	if strings.Contains(body, "outcome") || strings.Contains(body, "a.go") {
		t.Errorf("the fields are in the fenced body: %q", body)
	}
	plain, _, _ := strings.Cut(Format(Message{ID: 8, From: "side-1", To: "track", Kind: Report, Body: "x", At: time.Unix(0, 0)}, true), "\n")
	if strings.Contains(plain, "outcome") || strings.Contains(plain, "refs") {
		t.Errorf("a message without fields: %q", plain)
	}
}
