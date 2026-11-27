package cli

import (
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/relay"
	"github.com/bluegardenproject/tracks/internal/rpc"
)

func TestReadMessage(t *testing.T) {
	for in, ok := range map[string]bool{
		"Why?":                               true,
		"héllo ✓":                            true,
		strings.Repeat("a", relay.MaxBody):   true,
		strings.Repeat("a", relay.MaxBody+1): false,
		"bad \xff byte":                      false,
	} {
		got, err := readMessage(strings.NewReader(in))
		if (err == nil) != ok || (ok && got != in) {
			t.Errorf("readMessage(%.20q) = %.20q, %v; want ok %v", in, got, err, ok)
		}
	}
}

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

func TestRelaySendFlags(t *testing.T) {
	cmd := newRelaySendCmd()
	if err := cmd.ParseFlags([]string{"--to", "track", "--kind", "report", "--outcome", "pass",
		"--ref", "#12", "--ref", "a,b.go"}); err != nil {
		t.Fatal(err)
	}
	if o, _ := cmd.Flags().GetString("outcome"); o != "pass" {
		t.Errorf("--outcome %q", o)
	}
	if refs, _ := cmd.Flags().GetStringArray("ref"); len(refs) != 2 || refs[1] != "a,b.go" {
		t.Errorf("--ref %q, want each as given, commas kept", refs)
	}
}
