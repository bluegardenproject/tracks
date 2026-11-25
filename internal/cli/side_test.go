package cli

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/review"
	"github.com/bluegardenproject/tracks/internal/rpc"
	"github.com/bluegardenproject/tracks/internal/track"
)

func TestSideRequest(t *testing.T) {
	req, err := sideRequest(sideNewFlags{task: "x", name: "relay-store", repos: []string{"api"}}, " Store the relay\n")
	if err != nil {
		t.Fatal(err)
	}
	if !req.Side || req.Kind != track.Work || req.Prompt != "Store the relay" || req.Name != "relay-store" || !req.NoDraft ||
		len(req.Repos) != 1 || req.Parent != "" {
		t.Errorf("request %+v", req)
	}
	for _, f := range []sideNewFlags{{}, {task: "a", model: "opus"}} {
		if _, err := sideRequest(f, f.task); err == nil {
			t.Errorf("%+v made a request", f)
		}
	}
}

func TestSideNewTakesOneTask(t *testing.T) {
	t.Setenv("TRACKS_ID", "20261125-100000-aaaaaa")
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv(review.EnvMarker, "")
	cmd := newSideCmd()
	cmd.SetArgs([]string{"new", "--task", "x", "--task-file", "-"})
	cmd.SetIn(strings.NewReader("y"))
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "--task or --task-file") {
		t.Errorf("both --task and --task-file: %v, want an error naming them", err)
	}
	t.Setenv(review.EnvMarker, "1")
	cmd = newSideCmd()
	cmd.SetArgs([]string{"list"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	if err := cmd.Execute(); !errors.Is(err, review.ErrAlreadyReviewing) {
		t.Errorf("tracks side under the review marker: %v, want a refusal", err)
	}
	if got, err := readTask(strings.NewReader("From stdin"), sideNewFlags{taskFile: "-"}); err != nil || got != "From stdin" {
		t.Errorf("--task-file -: %q, %v", got, err)
	}
}

func TestPrintSides(t *testing.T) {
	var b bytes.Buffer
	if err := printSides(&b, nil); err != nil || b.String() != "No side tracks.\n" {
		t.Errorf("no side tracks: %q, %v", b.String(), err)
	}
	b.Reset()
	err := printSides(&b, []rpc.SideTrack{
		{Name: "relay-store", Handle: "side-000123", Status: "active"},
		{Name: "relay-cli", Handle: "side-000456", Status: "done", PR: "PR open", PRs: []string{"https://github.com/o/r/pull/231"}},
	})
	want := "NAME         HANDLE        STATUS  PR\n" +
		"relay-store  @side-000123  active  -\n" +
		"relay-cli    @side-000456  done    PR open https://github.com/o/r/pull/231\n"
	if err != nil || b.String() != want {
		t.Errorf("got\n%s\nwant\n%s", b.String(), want)
	}
}
