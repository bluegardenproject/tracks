package track

import (
	"testing"
	"time"
)

func TestFilterMatch(t *testing.T) {
	berlin := time.FixedZone("CEST", 2*3600)
	now := time.Date(2026, 9, 29, 18, 0, 0, 0, berlin)
	day := func(d, h int) time.Time { return time.Date(2026, 9, d, h, 0, 0, 0, berlin) }
	active := Track{CreatedAt: day(29, 9)}
	waiting := Track{CreatedAt: day(28, 9), State: State{Waiting: true}}
	done := Track{CreatedAt: day(23, 0), State: State{ClosedAt: day(24, 0)}, PRs: []PR{{State: PRMerged}}}
	closed := Track{CreatedAt: day(22, 23), State: State{ClosedAt: day(24, 0), CleanedAt: day(24, 0)}, PRs: []PR{{State: PRDraft}}}
	archived := Track{CreatedAt: day(1, 0), State: State{ClosedAt: day(2, 0), ArchivedAt: day(9, 0)}}

	tests := []struct {
		name   string
		f      Filter
		tracks map[string]bool
	}{
		{"none", Filter{}, map[string]bool{"active": true, "waiting": true, "done": true, "closed": true, "archived": false}},
		{"statuses", Filter{Statuses: []string{"done", "action_required"}}, map[string]bool{"waiting": true, "done": true, "active": false, "closed": false}},
		{"no PR", Filter{PRStatuses: []string{"none"}}, map[string]bool{"active": true, "done": false, "closed": false}},
		{"PR open counts drafts", Filter{PRStatuses: []string{"open", "merged"}}, map[string]bool{"done": true, "closed": true, "active": false}},
		{"archived only", Filter{Archived: true}, map[string]bool{"archived": true, "done": false, "active": false}},
		{"today", Filter{Started: Today}, map[string]bool{"active": true, "waiting": false}},
		{"last 7 days from local midnight", Filter{Started: Last7Days}, map[string]bool{"done": true, "closed": false}},
		{"between includes both days", Filter{Started: Between, From: "2026-09-23", To: "2026-09-28"}, map[string]bool{"done": true, "waiting": true, "closed": false, "active": false}},
		{"from only", Filter{Started: Between, From: "2026-09-28"}, map[string]bool{"waiting": true, "active": true, "done": false}},
		{"to only", Filter{Started: Between, To: "2026-09-22"}, map[string]bool{"closed": true, "done": false}},
	}
	all := map[string]Track{"active": active, "waiting": waiting, "done": done, "closed": closed, "archived": archived}
	for _, tt := range tests {
		for name, want := range tt.tracks {
			if got := tt.f.Match(all[name], now); got != want {
				t.Errorf("%s: Match(%s) = %v, want %v", tt.name, name, got, want)
			}
		}
	}
}

func TestFilterCheck(t *testing.T) {
	for f, want := range map[*Filter]string{
		{}: "",
		{Statuses: []string{"done"}, PRStatuses: []string{"none", "closed"}, Started: Last30Days}: "",
		{Started: Between, From: "2026-09-01", To: "2026-09-01"}:                                  "",
		{Statuses: []string{"archived"}}:                                                          "There's no track status archived.",
		{PRStatuses: []string{"draft"}}:                                                           "There's no PR status draft.",
		{Started: "week"}:                                                                         "There's no start week.",
		{Started: Between}:                                                                        "Enter a From or a To date.",
		{Started: Between, From: "29.09.2026"}:                                                    "From isn't a date like 2026-09-29.",
		{Started: Between, To: "2026-02-30"}:                                                      "To isn't a date like 2026-09-29.",
		{Started: Between, From: "2026-09-02", To: "2026-09-01"}:                                  "From is after To.",
	} {
		got := ""
		if err := f.Check(); err != nil {
			got = err.Error()
		}
		if got != want {
			t.Errorf("Check(%+v) = %q, want %q", *f, got, want)
		}
	}
}

func TestFilterString(t *testing.T) {
	for f, want := range map[*Filter]string{
		{}: "",
		{Statuses: []string{"closed", "done"}, PRStatuses: []string{"merged"}, Started: Last7Days}: "done, closed · PR merged · started in the last 7 days",
		{Archived: true, PRStatuses: []string{"closed", "none"}}:                                   "archived · no PR, PR closed",
		{Started: Between, From: "2026-09-01", To: "2026-09-20"}:                                   "started 2026-09-01 to 2026-09-20",
		{Started: Between, To: "2026-09-20"}:                                                       "started until 2026-09-20",
	} {
		if got := f.String(); got != want {
			t.Errorf("%+v = %q, want %q", *f, got, want)
		}
		if f.On() != (want != "") {
			t.Errorf("%+v: On = %v", *f, f.On())
		}
	}
}
