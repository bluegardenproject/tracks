package cli

import (
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/rpc"
)

func TestPrintMembers(t *testing.T) {
	var out strings.Builder
	err := printMembers(&out, []rpc.Member{
		{Handle: "track", Agent: "Track", Engine: "Claude Code (opus)", Status: "active"},
		{Handle: "reviewer-527192", Agent: "Reviewer", Engine: "Cursor (m1)", Status: "done", Outcome: "pass"},
		{Handle: "watch-000001", Agent: "Watcher", Status: "active"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "HANDLE            AGENT     ON                  STATUS\n" +
		"@track            Track     Claude Code (opus)  active\n" +
		"@reviewer-527192  Reviewer  Cursor (m1)         done, pass\n" +
		"@watch-000001     Watcher   -                   active\n"
	if out.String() != want {
		t.Errorf("got\n%s\nwant\n%s", out.String(), want)
	}
}
