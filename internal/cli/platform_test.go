package cli

import (
	"errors"
	"testing"

	"github.com/bluegardenproject/tracks/internal/rpc"
	"github.com/bluegardenproject/tracks/internal/track"
)

func TestPlatformSides(t *testing.T) {
	sides, err := platformSides([]rpc.SideTrack{
		{ID: "a", Name: "relay-store", Status: "active", StatusID: "active", PRID: "none"},
		{ID: "b", Name: "relay-cli", Title: "Relay CLI", Status: "done", StatusID: "done", PR: "2 PRs open", PRID: "open"},
		{ID: "c", Name: "later", Status: "?", StatusID: "new-status", PR: "PR queued", PRID: "queued"},
	}, nil)
	if err != nil || len(sides) != 3 {
		t.Fatalf("platformSides = %+v, %v", sides, err)
	}
	if s := sides[0]; s.ID != "a" || s.Status != track.Active || s.PR.Label != "" {
		t.Errorf("relay-store = %+v; want active, no PR badge", s)
	}
	if s := sides[1]; s.Name != "Relay CLI" || s.Status != track.Done || s.PR.Badge != track.PRsOpen.Badge || s.PR.Label != "2 PRs open" {
		t.Errorf("relay-cli = %+v; want its title, done, the PR badge with the daemon's label", s)
	}
	if s := sides[2]; s.Status.Label != "" || s.PR.Label != "" {
		t.Errorf("a newer daemon's statuses = %+v; want no badges for what this one doesn't know", s)
	}
	if _, err := platformSides(nil, errors.New("no daemon")); err == nil {
		t.Error("the daemon's error is dropped")
	}
}
