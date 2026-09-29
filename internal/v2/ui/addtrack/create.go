package addtrack

import (
	"context"
	"errors"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/v2/track"
	"github.com/bluegardenproject/tracks/internal/v2/tracks"
)

// CreateFunc creates the track req asks for, telling progress each slow
// step. Cancelling ctx hangs up; the track is still made.
type CreateFunc func(ctx context.Context, req tracks.Request, progress func(string)) (Created, error)

// Created is a track the form made, and its window.
type Created struct{ Name, Window string }

// creation is a Create under way.
type creation struct {
	events <-chan createEvent
	hangUp context.CancelFunc
}

// createEvent is a creation's progress, or its end when done.
type createEvent struct {
	progress string
	done     bool
	created  Created
	err      error
}

var trackKinds = map[Kind]track.Kind{Work: track.Work, Ask: track.Ask, Plan: track.Plan, Review: track.Review, Doc: track.Doc}

// request is the track the form describes.
func (m Model) request() tracks.Request {
	r := tracks.Request{
		Kind: trackKinds[m.kind], Name: strings.TrimSpace(m.name.Value()), Prompt: m.prompt.Value(),
		Terminal: m.terminal && m.kind == Work,
	}
	switch m.kind {
	case Review:
		r.Repos = []string{m.repos[m.repo]}
		r.ReviewRef = strings.TrimSpace(m.target.Value())
		r.Candor = m.candor
	case Doc:
		r.Repos = m.pickedRepos()
		// The daemon runs in another folder, so it gets the path resolved.
		r.Document, _ = tracks.ResolveDocument(m.document.Value())
		r.Candor, r.Opinion, r.ClaimCheck = m.candor, m.sections[0], m.sections[1]
	default:
		r.Repos = m.pickedRepos()
	}
	return r
}

// startCreate sends the request and waits for the first event.
func (m Model) startCreate() (Model, tea.Cmd) {
	if _, ok := m.runsOn[trackKinds[m.kind]]; !ok || m.create == nil {
		m.notice = string(tracks.ErrNoEngine)
		return m, nil
	}
	events := make(chan createEvent, 16)
	ctx, hangUp := context.WithCancel(context.Background())
	create, req := m.create, m.request()
	go func() {
		created, err := create(ctx, req, func(s string) { events <- createEvent{progress: s} })
		events <- createEvent{done: true, created: created, err: err}
	}()
	m.creating = &creation{events: events, hangUp: hangUp}
	m.failure, m.notice = "", "Creating the track…"
	return m, m.creating.next()
}

func (c *creation) next() tea.Cmd {
	return func() tea.Msg { return <-c.events }
}

// created handles a creation's event: progress shows in the hint row,
// a failure stays on the form, and a new track closes it.
func (m Model) created(e createEvent) (Model, tea.Cmd) {
	if m.creating == nil {
		return m, nil
	}
	if !e.done {
		m.notice = e.progress
		return m, m.creating.next()
	}
	m.creating.hangUp()
	m.creating, m.notice = nil, ""
	if e.err != nil {
		m.failure = e.err.Error()
		var p tracks.Problem
		if !errors.As(e.err, &p) {
			m.failure = "Couldn't create the track: " + m.failure
		}
		return m, nil
	}
	m.made = &e.created
	return m, tea.Quit
}

// creatingKey is a key while the track is being made: Esc and Ctrl+C
// close the form, and creation goes on.
func (m Model) creatingKey(key string) (Model, tea.Cmd) {
	switch key {
	case "esc", "ctrl+c":
		m.creating.hangUp()
		return m, tea.Quit
	}
	return m, nil
}

// Made is the track the form created, nil when it closed without one.
func (m Model) Made() *Created { return m.made }

// engineLine says what runs the track.
func (m Model) engineLine() string {
	on, ok := m.runsOn[trackKinds[m.kind]]
	if !ok {
		return ""
	}
	line := "Runs on " + on.Engine
	if on.Missing {
		return line + ", which isn't added on the Engines tab."
	}
	if on.Model != "" {
		line += ", model " + on.Model
	}
	return line + "."
}
