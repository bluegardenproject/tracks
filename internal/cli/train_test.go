package cli

import (
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/rpc"
)

func TestPrintTrains(t *testing.T) {
	var none strings.Builder
	if err := printTrains(&none, nil); err != nil || none.String() != "No trains on this track.\n" {
		t.Errorf("no trains: %q, %v", none.String(), err)
	}
	var out strings.Builder
	err := printTrains(&out, []rpc.Train{
		{Handle: "reviewer", Agent: "Reviewer", Engine: "Cursor (m1)", Status: "done", Outcome: "blocked"},
		{Handle: "detective", Agent: "Investigator", Status: "active"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "NAME       AGENT         ON           STATUS\n" +
		"reviewer   Reviewer      Cursor (m1)  done, blocked\n" +
		"detective  Investigator  -            active\n"
	if out.String() != want {
		t.Errorf("got\n%s\nwant\n%s", out.String(), want)
	}
	out.Reset()
	err = printTrains(&out, []rpc.Train{
		{Handle: "reviewer-1", Agent: "Reviewer", Engine: "Claude", Status: "active", Target: "/w/api"},
		{Handle: "reviewer-2", Agent: "Reviewer", Engine: "Claude", Status: "active", Target: "/w/api-fix"},
		{Handle: "detective", Agent: "Investigator", Status: "active"},
	})
	want = "NAME        AGENT         ON      STATUS  REVIEWS\n" +
		"reviewer-1  Reviewer      Claude  active  /w/api\n" +
		"reviewer-2  Reviewer      Claude  active  /w/api-fix\n" +
		"detective   Investigator  -       active  -\n"
	if err != nil || out.String() != want {
		t.Errorf("with targets: got\n%s\nwant\n%s", out.String(), want)
	}
}

func TestPlatformEntries(t *testing.T) {
	got, err := platformEntries([]rpc.Train{{Handle: "reviewer-1", StatusID: "done", OutcomeID: "pass"}, {Handle: "w-1", StatusID: "active"}}, nil)
	if err != nil || len(got) != 2 || got[0].Status.ID != "done" || got[0].Outcome.ID != "pass" || got[1].Outcome.Label != "" {
		t.Errorf("entries %+v, %v", got, err)
	}
}
