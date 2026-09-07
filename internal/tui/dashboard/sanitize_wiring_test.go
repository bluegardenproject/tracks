package dashboard

import (
	"errors"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/state"
)

// escapePayload is a plausible hostile string: retitle the window, write
// the clipboard via OSC 52, clear the screen, and ask the terminal to
// report its cursor position (which some terminals answer by typing the
// reply into the foreground process's stdin).
const escapePayload = "\x1b]0;retitled\x07\x1b]52;c;cGF5bG9hZA==\x07\x1b[2J\x1b[6n"

// hasEscapeIntroducer reports whether s carries anything the terminal
// would read as the start of a control sequence.
func hasEscapeIntroducer(s string) bool {
	return strings.ContainsAny(s, "\x1b\x07")
}

// The panel's ERROR / INTERRUPTED / DRAFT sections and the task prompt
// all reach the screen through wrapInfoText, so that is the application
// point for every one of them. ErrorMsg is the valuable case: it quotes
// git and gh stderr, which quote the remote.
func TestWrapInfoTextStripsEscapes(t *testing.T) {
	for _, width := range []int{80, 1, 0, -1} {
		got := strings.Join(wrapInfoText("fetch failed: "+escapePayload, width), "\n")
		if hasEscapeIntroducer(got) {
			t.Errorf("width %d: wrapInfoText kept an escape introducer: %q", width, got)
		}
	}
}

// A width of zero or less takes an early return, which must not be a way
// around the strip.
func TestWrapInfoTextStripsAtDegenerateWidth(t *testing.T) {
	got := wrapInfoText(escapePayload, 0)
	if len(got) != 1 {
		t.Fatalf("wrapInfoText returned %d lines, want 1", len(got))
	}
	if hasEscapeIntroducer(got[0]) {
		t.Errorf("kept an escape introducer: %q", got[0])
	}
}

// PR URLs are filtered at ingest, but a URL persisted by an older build
// predates that filter — so the render site strips too.
func TestRenderDetailStripsEscapesFromPRURL(t *testing.T) {
	m := &model{}
	d := detail{track: state.Track{
		ID:     "a",
		Status: state.StatusPROpen,
		PRs:    []state.PRRef{{URL: "https://github.com/o/r/pull/1" + escapePayload}},
	}}
	got := m.renderDetail(d, 120, -1)
	if hasEscapeIntroducer(got) {
		t.Errorf("renderDetail kept an escape introducer from a PR URL:\n%q", got)
	}
	// Guard against the assertion passing because the URL never reached
	// the render path at all.
	if !strings.Contains(got, "github.com/o/r/pull/1") {
		t.Fatalf("the PR URL was not rendered, so this test proves nothing:\n%s", got)
	}
}

// The same text that fills the panel's ERROR section also lands in the
// one-line status message above the table, which is a separate render
// site with the same provenance.
func TestRenderStatusMessageStripsEscapes(t *testing.T) {
	m := &model{
		statusMsg: "could not end track: " + escapePayload,
		tracks:    []state.Track{{ID: "a"}},
		width:     120,
		height:    40,
	}
	got := m.View()
	if hasEscapeIntroducer(got) {
		t.Errorf("View kept an escape introducer from statusMsg:\n%q", got)
	}
	if !strings.Contains(got, "could not end track") {
		t.Fatalf("the status message was not rendered, so this test proves nothing:\n%s", got)
	}
}

// The daemon-unreachable line prints the error's own text, which comes
// from the daemon over the socket.
func TestRenderDaemonErrorStripsEscapes(t *testing.T) {
	m := &model{
		err:    errors.New("dial daemon: " + escapePayload),
		width:  120,
		height: 40,
	}
	got := m.View()
	if hasEscapeIntroducer(got) {
		t.Errorf("View kept an escape introducer from m.err:\n%q", got)
	}
	if !strings.Contains(got, "dial daemon") {
		t.Fatalf("the daemon error was not rendered, so this test proves nothing:\n%s", got)
	}
}

// A slug reaches the table and the panel title. For a doc-review track
// it is derived from a filename on disk, so it is not the user's typing.
func TestRenderStripsEscapesFromSlugAndBranch(t *testing.T) {
	track := state.Track{
		ID:     "abcdef123456",
		Slug:   "review" + escapePayload,
		Branch: "fix/thing" + escapePayload,
		Status: state.StatusRunning,
	}
	m := &model{tracks: []state.Track{track}, width: 200, height: 40}
	if got := m.View(); hasEscapeIntroducer(got) {
		t.Errorf("View kept an escape introducer from the slug or branch:\n%q", got)
	}
	d := detail{track: track}
	if got := m.renderDetail(d, 120, -1); hasEscapeIntroducer(got) {
		t.Errorf("renderDetail kept an escape introducer from the slug or branch:\n%q", got)
	}
}
