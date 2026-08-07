package daemon

import "testing"

// A track that opens a stack of PRs emits one TRACKS_PR_URL marker per
// PR, and the pane usually still shows the earlier ones — so the scan
// has to return every distinct URL, in order, and ignore the `none`
// sentinel and the `<url>` placeholder from the instruction text.
func TestScanForPRURLs(t *testing.T) {
	cases := []struct {
		name     string
		snapshot string
		want     []string
	}{
		{"no marker", "just some output\n", nil},
		{"placeholder only",
			"as `TRACKS_PR_URL=<url>` so the tracks dashboard surfaces it",
			nil},
		{"sentinel none", "TRACKS_PR_URL=none\n", nil},
		{"mentioned mid-sentence is not a marker",
			"I told it to emit TRACKS_PR_URL=https://github.com/o/r/pull/9 on its own line\n",
			nil},
		{"TUI bullet before the marker",
			"⏺ TRACKS_PR_URL=https://github.com/o/r/pull/5\n",
			[]string{"https://github.com/o/r/pull/5"}},
		{"indented continuation line",
			"⏺ Opened the PR.\n  TRACKS_PR_URL=https://github.com/o/r/pull/6\n",
			[]string{"https://github.com/o/r/pull/6"}},
		{"one url",
			"done.\nTRACKS_PR_URL=https://github.com/o/r/pull/1\n",
			[]string{"https://github.com/o/r/pull/1"}},
		{"three urls in order",
			"TRACKS_PR_URL=https://github.com/o/r/pull/1\n" +
				"more output\n" +
				"TRACKS_PR_URL=https://github.com/o/r/pull/2\n" +
				"TRACKS_PR_URL=https://github.com/o/r/pull/3\n",
			[]string{
				"https://github.com/o/r/pull/1",
				"https://github.com/o/r/pull/2",
				"https://github.com/o/r/pull/3",
			}},
		{"repeated marker de-duplicated",
			"TRACKS_PR_URL=https://github.com/o/r/pull/1\n" +
				"TRACKS_PR_URL=https://github.com/o/r/pull/1\n",
			[]string{"https://github.com/o/r/pull/1"}},
		{"none alongside a real url",
			"TRACKS_PR_URL=none\nTRACKS_PR_URL=https://github.com/o/r/pull/4\n",
			[]string{"https://github.com/o/r/pull/4"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := scanForPRURLs(c.snapshot)
			if len(got) != len(c.want) {
				t.Fatalf("scanForPRURLs() = %v, want %v", got, c.want)
			}
			for i := range c.want {
				if got[i] != c.want[i] {
					t.Errorf("scanForPRURLs()[%d] = %q, want %q", i, got[i], c.want[i])
				}
			}
		})
	}
}
