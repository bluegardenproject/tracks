package track

import (
	"testing"
	"time"
)

func TestApply(t *testing.T) {
	then, now := time.UnixMilli(1000), time.UnixMilli(2000)
	var (
		active  = State{}
		waiting = State{Waiting: true}
		done    = State{ClosedAt: then}
		closed  = State{ClosedAt: then, CleanedAt: then}
	)
	tests := []struct {
		from State
		e    Event
		want State
	}{
		{active, Created, active},
		{active, AgentWaiting, waiting},
		{waiting, AgentWaiting, waiting},
		{waiting, AgentWorking, active},
		{active, AgentWorking, active},
		{active, Ended, State{ClosedAt: now}},
		{waiting, Ended, State{ClosedAt: now}},
		{done, Ended, done},
		{closed, Ended, closed},
		{active, Cleaned, active},
		{done, Cleaned, State{ClosedAt: then, CleanedAt: now}},
		{closed, Cleaned, closed},
		{done, Resumed, active},
		{closed, Resumed, active},
		{done, AgentWaiting, done},
		{closed, AgentWorking, closed},
		{active, "agent.dancing", active},
		{active, Archived, active},
		{waiting, Archived, waiting},
		{done, Archived, State{ClosedAt: then, ArchivedAt: now}},
		{closed, Archived, State{ClosedAt: then, CleanedAt: then, ArchivedAt: now}},
		{State{ClosedAt: then, ArchivedAt: then}, Archived, State{ClosedAt: then, ArchivedAt: then}},
		{State{ClosedAt: then, ArchivedAt: then}, Unarchived, done},
		{State{ClosedAt: then, ArchivedAt: then}, Cleaned, State{ClosedAt: then, CleanedAt: now, ArchivedAt: then}},
		{State{ClosedAt: then, ArchivedAt: then}, Resumed, active},
		{done, Unarchived, done},
	}
	for _, tt := range tests {
		if got := tt.from.Apply(tt.e, now); got != tt.want {
			t.Errorf("%s on %+v = %+v, want %+v", tt.e, tt.from, got, tt.want)
		}
	}
}

func TestStatusOf(t *testing.T) {
	then := time.UnixMilli(1000)
	for s, want := range map[State]Status{
		{}:                                 Active,
		{Waiting: true}:                    ActionRequired,
		{ClosedAt: then}:                   Done,
		{ClosedAt: then, CleanedAt: then}:  Closed,
		{ClosedAt: then, Waiting: true}:    Done,
		{ClosedAt: then, ArchivedAt: then}: Done,
	} {
		if got := s.Status(); got != want {
			t.Errorf("%+v is %s, want %s", s, got.ID, want.ID)
		}
	}
}

func TestStatuses(t *testing.T) {
	ids := map[string]bool{}
	for i, s := range Statuses {
		if s.ID == "" || s.Label == "" || s.Badge == "" || ids[s.ID] {
			t.Errorf("%+v needs a unique ID, a label and a badge", s)
		}
		ids[s.ID] = true
		if i > 0 && s.Priority <= Statuses[i-1].Priority {
			t.Errorf("Statuses should be in priority order: %s after %s", s.ID, Statuses[i-1].ID)
		}
	}
	if !ActionRequired.Attention || Active.Attention || Done.Attention || Closed.Attention {
		t.Error("only action required needs attention")
	}
}
