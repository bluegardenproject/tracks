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
}
