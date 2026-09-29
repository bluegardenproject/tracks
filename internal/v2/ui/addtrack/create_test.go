package addtrack

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
	"github.com/bluegardenproject/tracks/internal/v2/track"
	"github.com/bluegardenproject/tracks/internal/v2/tracks"
)

// ready is a Work form for repo tracks with a prompt, on Claude Code.
func ready(t *testing.T, create CreateFunc) Model {
	m, _ := send(New(Config{Theme: theme.Default(), Repos: []string{"tracks"}, Engine: "Claude Code", Model: "opus", Create: create}), resize)
	m = m.setFocus(ctlRepos)
	m, _ = send(m, space, space, enter)
	m = m.setFocus(ctlPrompt)
	m, _ = send(m, typed("Fix the rate bug")...)
	return m
}

// pressCreate presses Create and returns the command waiting for the
// creation's first event.
func pressCreate(t *testing.T, m Model) (Model, tea.Cmd) {
	t.Helper()
	m = m.setFocus(ctlCreate)
	m, cmd := send(m, enter)
	if m.creating == nil || cmd == nil {
		t.Fatalf("Create didn't start: problems %v, notice %q", m.errs, m.notice)
	}
	return m, cmd
}

// step delivers the creation's next event.
func step(m Model, cmd tea.Cmd) (Model, tea.Cmd) { return send(m, cmd()) }

func TestCreateSendsTheRequest(t *testing.T) {
	var got tracks.Request
	m := ready(t, func(_ context.Context, req tracks.Request, progress func(string)) (Created, error) {
		got = req
		progress("Fetching origin/main in tracks…")
		return Created{Name: "fix-the-rate-bug", Window: "@4"}, nil
	})
	if lines, _ := m.body(90); !strings.Contains(strings.Join(lines, "\n"), "Runs on Claude Code, model opus.") {
		t.Error("the form doesn't say what runs the track")
	}
	m, cmd := pressCreate(t, m)
	m, cmd = step(m, cmd)
	if !strings.Contains(m.hints(), "Fetching origin/main in tracks…") {
		t.Errorf("hint row %q doesn't show the progress", m.hints())
	}
	m, cmd = step(m, cmd)
	if !quits(cmd) || m.Made() == nil || *m.Made() != (Created{Name: "fix-the-rate-bug", Window: "@4"}) {
		t.Fatalf("after creating: quit %v, made %v", quits(cmd), m.Made())
	}
	if got.Kind != track.Work || len(got.Repos) != 1 || got.Repos[0] != "tracks" || got.Prompt != "Fix the rate bug" {
		t.Errorf("request = %+v", got)
	}
}

func TestCreateFailureKeepsTheForm(t *testing.T) {
	for err, want := range map[error]string{
		errors.New("git fetch failed"):   "Couldn't create the track: git fetch failed",
		tracks.Problem("Pick one repo."): "Pick one repo.",
	} {
		m := ready(t, func(context.Context, tracks.Request, func(string)) (Created, error) { return Created{}, err })
		m, cmd := pressCreate(t, m)
		m, cmd = step(m, cmd)
		if quits(cmd) || m.creating != nil || m.failure != want {
			t.Errorf("after %v: quit %v, creating %v, failure %q", err, quits(cmd), m.creating != nil, m.failure)
		}
		if m.Made() != nil {
			t.Error("a failed creation made a track")
		}
	}
}

func TestCreateWithoutAnEngine(t *testing.T) {
	m := ready(t, nil)
	m.engine = ""
	m = m.setFocus(ctlCreate)
	m, cmd := send(m, enter)
	if m.creating != nil || cmd != nil || m.notice != string(tracks.ErrNoEngine) {
		t.Errorf("creating %v, notice %q", m.creating != nil, m.notice)
	}
}

func TestEscWhileCreatingClosesAndHangsUp(t *testing.T) {
	hungUp := make(chan struct{})
	m := ready(t, func(ctx context.Context, _ tracks.Request, _ func(string)) (Created, error) {
		<-ctx.Done()
		close(hungUp)
		return Created{}, ctx.Err()
	})
	m, _ = pressCreate(t, m)
	if m, cmd := send(m, typed("x")...); m.prompt.Value() != "Fix the rate bug" || cmd != nil {
		t.Error("the form took a key while creating")
	}
	if _, cmd := send(m, esc); !quits(cmd) {
		t.Fatal("Esc didn't close the form")
	}
	select {
	case <-hungUp:
	case <-time.After(5 * time.Second):
		t.Fatal("closing didn't hang up")
	}
}
