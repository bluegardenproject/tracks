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
		{"starting", ReviewerFacts{Pane: true}, track.Active, track.Status{}, true},
		{"asks trust", ReviewerFacts{Pane: true, AsksTrust: true}, track.ActionRequired, track.Status{}, true},
		{"report in", ReviewerFacts{Pane: true, Report: report}, track.Done, Blocked, true},
		{"pane gone, report in", ReviewerFacts{Report: report}, track.Status{}, track.Status{}, false},
		{"pane gone, not run", ReviewerFacts{Report: "REVIEW OUTCOME: not run"}, track.Status{}, track.Status{}, false},
		{"answering a question", ReviewerFacts{Pane: true, Report: report, Asking: true}, track.Active, Blocked, true},
		{"not run", ReviewerFacts{Pane: true, Report: "The turn failed.\nREVIEW OUTCOME: not run\n"}, track.Error, track.Status{}, true},
		{"exited without a report", ReviewerFacts{Pane: true, Exited: true}, track.Exited, track.Status{}, true},
		{"exited after its report", ReviewerFacts{Pane: true, Exited: true, Report: report}, track.Done, Blocked, true},
		{"exited on a question", ReviewerFacts{Pane: true, Exited: true, Report: report, Asking: true}, track.Exited, Blocked, true},
		{"exited, not run", ReviewerFacts{Pane: true, Exited: true, Report: "REVIEW OUTCOME: not run"}, track.Error, track.Status{}, true},
		{"report quotes the trust question", ReviewerFacts{Pane: true, Report: report, AsksTrust: true}, track.Done, Blocked, true},
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

func TestStatusOf(t *testing.T) {
	for _, s := range []track.Status{track.Active, track.ActionRequired, track.Done, track.Exited, track.Error, Pass, Blocked} {
		if got := StatusOf(s.ID); got != s {
			t.Errorf("StatusOf(%q) = %+v", s.ID, got)
		}
	}
	if got := StatusOf("nope"); got != (track.Status{}) {
		t.Errorf("an unknown ID: %+v", got)
	}
}
