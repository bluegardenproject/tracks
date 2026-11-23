package train

import (
	"testing"

	"github.com/bluegardenproject/tracks/internal/track"
)

func TestCheckHandle(t *testing.T) {
	for h, ok := range map[string]bool{
		"reviewer": true, "watch-ci": true, "a": true, "a1": true, "abcdefghijklmnopqrstuvwxyz012345": true,
		"": false, "1a": false, "-a": false, "Big-Boss": false, "a b": false, "a_b": false, "@a": false,
		"abcdefghijklmnopqrstuvwxyz0123456": false,
	} {
		if err := CheckHandle(h); (err == nil) != ok {
			t.Errorf("CheckHandle(%q) = %v, want ok %v", h, err, ok)
		}
	}
}

func TestReviewerStatus(t *testing.T) {
	report := "Findings.\nREVIEW OUTCOME: blocked"
	for _, tc := range []struct {
		name            string
		f               ReviewerFacts
		status, outcome track.Status
		ok              bool
	}{
		{"none", ReviewerFacts{}, track.Status{}, track.Status{}, false},
		{"starting", ReviewerFacts{Pane: true, Run: true}, track.Active, track.Status{}, true},
		{"asks trust", ReviewerFacts{Pane: true, Run: true, AsksTrust: true}, track.ActionRequired, track.Status{}, true},
		{"report in", ReviewerFacts{Pane: true, Run: true, Report: report}, track.Done, Blocked, true},
		{"exited after", ReviewerFacts{Run: true, Report: report}, track.Exited, Blocked, true},
		{"exited before", ReviewerFacts{Run: true}, track.Exited, track.Status{}, true},
		{"pane, run gone", ReviewerFacts{Pane: true}, track.Active, track.Status{}, true},
		{"answering a question", ReviewerFacts{Pane: true, Run: true, Report: report, Asking: true}, track.Active, Blocked, true},
		{"not run", ReviewerFacts{Pane: true, Run: true, Report: "The turn failed.\nREVIEW OUTCOME: not run\n"}, track.Error, track.Status{}, true},
		{"report quotes the trust question", ReviewerFacts{Pane: true, Run: true, Report: report, AsksTrust: true}, track.Done, Blocked, true},
	} {
		status, outcome, ok := ReviewerStatus(tc.f)
		if status != tc.status || outcome != tc.outcome || ok != tc.ok {
			t.Errorf("%s: %s %s %v, want %s %s %v", tc.name, status.ID, outcome.ID, ok, tc.status.ID, tc.outcome.ID, tc.ok)
		}
	}
}

func TestOutcomeOf(t *testing.T) {
	for report, want := range map[string]track.Status{
		"REVIEW OUTCOME: pass":                          Pass,
		"x\n  REVIEW OUTCOME: Blocked  \n":              Blocked,
		"REVIEW OUTCOME: pass\nREVIEW OUTCOME: blocked": Blocked,
		"REVIEW OUTCOME: maybe":                         {},
		"no outcome":                                    {},
		"":                                              {},
	} {
		if got := OutcomeOf(report); got != want {
			t.Errorf("OutcomeOf(%q) = %q, want %q", report, got.ID, want.ID)
		}
	}
}

func TestAgentName(t *testing.T) {
	if got := Reviewer.Name(); got != "Reviewer" {
		t.Errorf("Reviewer.Name() = %q", got)
	}
}
