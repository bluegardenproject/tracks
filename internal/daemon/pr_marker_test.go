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
		// The marker is scanned out of a pane the agent doesn't fully
		// control, and every URL adopted here gets polled with `gh pr
		// view` and shown as one of the track's PRs.
		{"a non-github host is not adopted",
			"TRACKS_PR_URL=https://evil.test/o/r/pull/1\n",
			nil},
		{"a host that merely ends in github.com is not adopted",
			"TRACKS_PR_URL=https://github.com.evil.test/o/r/pull/1\n",
			nil},
		{"a github subdomain is fine",
			"TRACKS_PR_URL=https://www.github.com/o/r/pull/8\n",
			[]string{"https://www.github.com/o/r/pull/8"}},
		{"a real url still wins after a rejected one",
			"TRACKS_PR_URL=https://evil.test/x\nTRACKS_PR_URL=https://github.com/o/r/pull/7\n",
			[]string{"https://github.com/o/r/pull/7"}},
		// An escape can't ride along into state.json or the dashboard:
		// the capture stops at it. The URL in front of it is still a
		// real PR, so it is kept rather than the whole line discarded.
		{"an escape is excluded from the captured url",
			"TRACKS_PR_URL=https://github.com/o/r/pull/1\x1b[2Jgarbage\n",
			[]string{"https://github.com/o/r/pull/1"}},
		// U+009B is a CSI introducer on terminals that still decode it,
		// and Go's `\s`/`\x7f` classes don't reach it.
		{"a c1 introducer is excluded from the captured url",
			"TRACKS_PR_URL=https://github.com/o/r/pull/2\u009b2Jgarbage\n",
			[]string{"https://github.com/o/r/pull/2"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, _ := scanForPRURLs(c.snapshot)
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
