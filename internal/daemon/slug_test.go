package daemon

import "testing"

func TestDeriveSlugFromTask(t *testing.T) {
	cases := []struct {
		task     string
		fallback string
		want     string
	}{
		{
			task: "Solve LIVE-1234, the swap rate is wrong",
			want: "LIVE-1234-solve-swap-rate-wrong",
		},
		{
			task: "Add a tooltip to the send screen",
			want: "add-tooltip-send-screen",
		},
		{
			task: "LIVE-9999",
			want: "LIVE-9999",
		},
		{
			task: "fix the rate calc",
			want: "fix-rate-calc",
		},
		{
			task: "Please could you investigate ABC-42 and propose a fix",
			want: "ABC-42-investigate-propose-fix",
		},
		{
			// Stopwords-only after the ticket — should still return
			// just the ticket.
			task: "Look at LIVE-1 the the and the",
			want: "LIVE-1-look",
		},
		{
			task:     "",
			fallback: "abc123",
			want:     "abc123",
		},
		{
			// Very long prose: stops at the last whole word that
			// fits within maxSlugLength rather than mid-word.
			task: "investigate performance regression introduced in last release affecting webview rendering speed on android",
			want: "investigate-performance-regression",
		},
		{
			// Punctuation, quotes — survives.
			task: `Fix "broken" link in <Footer> component!!!`,
			want: "fix-broken-link-footer-component",
		},
	}
	for _, c := range cases {
		got := deriveSlugFromTask(c.task, c.fallback)
		if got != c.want {
			t.Errorf("deriveSlugFromTask(%q) = %q, want %q", c.task, got, c.want)
		}
	}
}
