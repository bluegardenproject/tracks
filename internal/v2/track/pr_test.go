package track

import "testing"

func TestParsePR(t *testing.T) {
	want := PR{URL: "https://github.com/acme/web.app/pull/42", Repo: "acme/web.app", Number: 42, State: PROpen}
	for _, url := range []string{
		"https://github.com/acme/web.app/pull/42",
		"https://github.com/acme/web.app/pull/42/files",
		"https://github.com/acme/web.app/pull/42#issuecomment-1",
		"https://github.com/acme/web.app/pull/42?w=1",
	} {
		if got, ok := ParsePR(url); !ok || got != want {
			t.Errorf("ParsePR(%q) = %+v, %v", url, got, ok)
		}
	}
	for _, url := range []string{
		"", "feat/x", "https://github.com/acme/web/issues/42", "https://github.com/acme/web/pull/0",
		"https://github.com/acme/web/pull/42x", "http://github.com/acme/web/pull/42", "see https://github.com/acme/web/pull/42",
	} {
		if got, ok := ParsePR(url); ok {
			t.Errorf("ParsePR(%q) = %+v, want none", url, got)
		}
	}
}

func TestPRStatus(t *testing.T) {
	pr := func(s PRState) PR { return PR{State: s} }
	tests := []struct {
		prs       []PR
		id, label string
	}{
		{nil, "none", ""},
		{[]PR{pr(PROpen)}, "open", "PR open"},
		{[]PR{pr(PRDraft)}, "open", "PR open"},
		{[]PR{pr(PROpen), pr(PRDraft), pr(PRMerged)}, "open", "2 PRs open"},
		{[]PR{pr(PRMerged), pr(PROpen)}, "open", "PR open"},
		{[]PR{pr(PRMerged)}, "merged", "PR merged"},
		{[]PR{pr(PRMerged), pr(PRClosed), pr(PRMerged)}, "merged", "PRs merged"},
		{[]PR{pr(PRClosed)}, "closed", "PR closed"},
		{[]PR{pr(PRClosed), pr(PRClosed)}, "closed", "PRs closed"},
	}
	for _, tt := range tests {
		if got := PRStatus(tt.prs); got.ID != tt.id || got.Label != tt.label {
			t.Errorf("PRStatus(%v) = %s %q, want %s %q", tt.prs, got.ID, got.Label, tt.id, tt.label)
		}
	}
}

func TestPRStatuses(t *testing.T) {
	for i, s := range PRStatuses {
		if s.ID == "" || s.Label == "" || s.Token == "" || s.Attention {
			t.Errorf("%+v needs an ID, a label and a token, and no attention", s)
		}
		if i > 0 && s.Priority <= PRStatuses[i-1].Priority {
			t.Errorf("PRStatuses should be in priority order: %s after %s", s.ID, PRStatuses[i-1].ID)
		}
	}
	if !PRMerged.Settled() || !PRClosed.Settled() || PROpen.Settled() || PRDraft.Settled() {
		t.Error("only merged and closed PRs are settled")
	}
}
