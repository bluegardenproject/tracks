package review

import (
	"slices"
	"strings"
	"testing"
)

func TestSplitDiff(t *testing.T) {
	diff := "diff --git a/x.go b/x.go\n--- a/x.go\n+++ b/x.go\n+1\n" +
		"diff --git a/my file b/my file\n+2\n" +
		"diff --git a/old b/new\nsimilarity index 90%\n" +
		"diff --git \"a/caf\\303\\251 \\\"x\\\".go\" \"b/caf\\303\\251 \\\"x\\\".go\"\n+3\n"
	got := SplitDiff(diff, true)
	var paths []string
	for _, d := range got {
		paths = append(paths, d.Path)
		if !d.Uncommitted || !strings.HasPrefix(d.Text, "diff --git ") {
			t.Errorf("part %+v", d)
		}
	}
	if !slices.Equal(paths, []string{"x.go", "my file", "new", "café \"x\".go"}) {
		t.Errorf("paths %q", paths)
	}
	if joined := got[0].Text + got[1].Text + got[2].Text + got[3].Text; joined != diff {
		t.Errorf("the parts don't add up to the diff:\n%s", joined)
	}
	if SplitDiff("", false) != nil {
		t.Error("an empty diff has parts")
	}
}

func TestParseAndMergeFiles(t *testing.T) {
	committed := ParseNumstat("3\t1\ta.go\x00-\t-\tlogo.png\x00")
	uncommitted := ParseNumstat("2\t0\ta.go\x001\t1\tb\tc.go\x00")
	got := MergeFiles(committed, uncommitted)
	want := []File{{Path: "a.go", Added: 5, Removed: 1}, {Path: "logo.png", Binary: true}, {Path: "b\tc.go", Added: 1, Removed: 1}}
	if !slices.Equal(got, want) {
		t.Errorf("files %+v, want %+v", got, want)
	}
}

func TestFit(t *testing.T) {
	diffs := []FileDiff{
		{Path: "a", Text: strings.Repeat("a", 40)},
		{Path: "big", Text: strings.Repeat("b", 100)},
		{Path: "c", Text: strings.Repeat("c", 50)},
		{Path: "big", Text: strings.Repeat("u", 30), Uncommitted: true},
	}
	kept, left := fitDiffs(diffs, 125)
	var paths []string
	for _, d := range kept {
		paths = append(paths, d.Path)
	}
	if !slices.Equal(paths, []string{"a", "c", "big"}) || !slices.Equal(left, []string{"big"}) {
		t.Errorf("kept %q, left %q; want the largest diff left out, whole, and the rest in order", paths, left)
	}
	if kept, left := fitDiffs(diffs, 1000); len(kept) != 4 || left != nil {
		t.Error("diffs within the limit were left out")
	}

	big := Packet{Diffs: []FileDiff{{Path: "a", Text: "x"}, {Path: "huge", Text: strings.Repeat("h", maxDiffBytes)}}}.Fit()
	if again := big.Fit(); len(again.Diffs) != 1 || !slices.Equal(again.Left, []string{"huge"}) {
		t.Errorf("fitting twice: %+v; want the file left out the first time still named", again.Left)
	}

	many := Packet{Files: make([]File, maxFiles+5)}.Fit()
	if len(many.Files) != maxFiles || many.MoreFiles != 5 || many.Fit().MoreFiles != 5 {
		t.Errorf("%d files listed, %d more; want the list's limit and the rest counted, once", len(many.Files), many.MoreFiles)
	}

	pr := Packet{PR: &PR{Body: strings.Repeat("b", maxPRBodyBytes+10),
		Checks:  []Check{{Log: strings.Repeat("l", maxLogBytes) + "THE END"}},
		Threads: []Thread{{Comments: []Comment{{Body: strings.Repeat("t", maxThreadsBytes-10)}}}, {Comments: []Comment{{Body: strings.Repeat("o", 20)}}}, {}},
	}}.Fit()
	if log := pr.PR.Checks[0].Log; !strings.HasSuffix(log, "THE END") || len(log) > maxLogBytes+maxNote {
		t.Error("a check's log wasn't cut to its end")
	}
	if pr.Fit().PR.Checks[0].Log != pr.PR.Checks[0].Log {
		t.Error("a cut log was cut again")
	}
	if len(pr.PR.Threads) != 1 || pr.PR.MoreThreads != 2 || !strings.Contains(pr.PR.Body, "[cut at") {
		t.Errorf("%d threads, %d more; want the threads over the limit counted", len(pr.PR.Threads), pr.PR.MoreThreads)
	}

	p := Packet{Commits: strings.Repeat("é", maxCommitsBytes), Brief: "short"}.Fit()
	if again := p.Fit(); again.Commits != p.Commits {
		t.Error("cut commit messages were cut again")
	}
	if len(p.Commits) > maxCommitsBytes+100 || !strings.Contains(p.Commits, "[cut at") || !strings.HasPrefix(p.Commits, "é") {
		t.Error("long commit messages weren't cut with a note")
	}
	if forged := cut(strings.Repeat("x", 100)+"\n\n[cut at 1 bytes of 2]", 10); len(forged) > 10+maxNote {
		t.Error("a long text ending like the note wasn't cut")
	}
	if p.Brief != "short" {
		t.Error("a short brief changed")
	}
}

func TestOwnedBy(t *testing.T) {
	c := ParseCodeowners("*.go @team\n/web/ @web")
	p := Packet{
		Files:     []File{{Path: "a.go"}, {Path: "web/x.ts"}, {Path: "web/y.go"}},
		Diffs:     []FileDiff{{Path: "a.go"}, {Path: "web/x.ts"}, {Path: "a.go", Uncommitted: true}},
		MoreFiles: 2,
	}.OwnedBy(c, "team")
	var files, diffs []string
	for _, f := range p.Files {
		if !f.Outside {
			files = append(files, f.Path)
		}
	}
	for _, d := range p.Diffs {
		diffs = append(diffs, d.Path)
	}
	if !slices.Equal(files, []string{"a.go"}) || !slices.Equal(diffs, []string{"a.go", "a.go"}) ||
		*p.Scope != (Scope{Owner: "@team", Owned: 1, Total: 5}) {
		t.Errorf("files %q, diffs %q, scope %+v; want @team's, of all five", files, diffs, p.Scope)
	}
	if len(p.Files) != 3 {
		t.Errorf("%d files listed; want the others too, outside the scope", len(p.Files))
	}
	withPR := Packet{PR: &PR{Threads: []Thread{{Path: "a.go"}, {Path: "web/x.ts"}, {}}}}.OwnedBy(c, "@team")
	if len(withPR.PR.Threads) != 2 {
		t.Errorf("threads %+v; want those on @team's files, and on none", withPR.PR.Threads)
	}
}
