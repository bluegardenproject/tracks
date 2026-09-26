package tracksview

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
	"github.com/bluegardenproject/tracks/internal/v2/ui/source"
)

const (
	// refreshEvery is how often Station reads the tracks again, so
	// tracks opened or closed elsewhere show up.
	refreshEvery = 2 * time.Second
	// doubleClick is the longest gap between two clicks on a row that
	// open its track.
	doubleClick = 400 * time.Millisecond
	// stationLeft is the margin left and right of the frames.
	stationLeft = 2
	// frameGap is the gap between the list and the details frames.
	frameGap = 1
	// detailsMin is the narrowest details frame; narrower windows show
	// the list alone.
	detailsMin = 34
	// fastTrackRows is the Fast Track frame's height, above the details.
	fastTrackRows = 10
)

type (
	// tracksMsg carries the tracks; poll schedules the next read.
	tracksMsg struct {
		tracks []source.Track
		err    error
		poll   bool
	}
	refreshMsg struct{}
	// doneMsg reports an action on a track: ok is shown on success.
	doneMsg struct {
		ok     string
		err    error
		reload bool
	}
)

// station is the Station tab's state.
type station struct {
	tracks     []source.Track
	err        error // reading the tracks failed
	selected   int
	offset     int // first row shown
	lastClick  time.Time
	hover      int  // the track under the mouse, -1 for none
	confirming bool // asking whether to end the selected track
	notice     notice
}

// notice is a short message in the hint row, until the next key or
// click.
type notice struct {
	text string
	err  bool
}

var columns = []string{"Slug", "Type", "Status", "Model", "Cost"}

// costColumn is right-aligned.
const costColumn = 4

func cells(t source.Track) []string {
	model, cost := "—", "—"
	if t.Model != "" {
		model = t.Model
	}
	if t.Cost > 0 {
		cost = fmt.Sprintf("$%.2f", t.Cost)
	}
	return []string{t.Name, t.Kind, t.Status, model, cost}
}

func (m Model) loadTracks(poll bool) tea.Cmd {
	if m.source == nil {
		return nil
	}
	src := m.source
	return func() tea.Msg {
		tracks, err := src.Tracks(context.Background())
		return tracksMsg{tracks, err, poll}
	}
}

func (m Model) setTracks(msg tracksMsg) Model {
	m.station.tracks, m.station.err = msg.tracks, msg.err
	m.station.selected = min(m.station.selected, max(0, len(msg.tracks)-1))
	return m.scrollStation()
}

func (m Model) selectedTrack() (source.Track, bool) {
	if len(m.station.tracks) == 0 {
		return source.Track{}, false
	}
	return m.station.tracks[m.station.selected], true
}

func (m Model) done(msg doneMsg) (Model, tea.Cmd) {
	switch {
	case msg.err != nil:
		m.station.notice = notice{msg.err.Error(), true}
	case msg.ok != "":
		m.station.notice = notice{text: msg.ok}
	}
	if msg.reload {
		return m, m.loadTracks(false)
	}
	return m, nil
}

// stationKey handles a key on the Station tab. ok is false for keys it
// doesn't use.
func (m Model) stationKey(key string) (_ Model, _ tea.Cmd, ok bool) {
	m.station.notice = notice{}
	if m.station.confirming {
		m.station.confirming = false
		switch key {
		case "y", "enter":
			return m, m.act(actionEnd), true
		case "n", "esc":
			return m, nil, true
		}
	}
	switch key {
	case "up", "k":
		m.station.selected = max(0, m.station.selected-1)
	case "down", "j":
		m.station.selected = min(max(0, len(m.station.tracks)-1), m.station.selected+1)
	default:
		for _, a := range actions {
			if a.key == key || (key == "enter" && a.id == actionOpen) {
				return m.press(a.id)
			}
		}
		return m, nil, false
	}
	return m.scrollStation(), nil, true
}

// stationClick handles a click on the list or the details. A second
// click on a row soon after the first opens its track.
func (m Model) stationClick(x, y int) (Model, tea.Cmd) {
	m.station.notice = notice{}
	if id, ok := m.buttonAt(x, y); ok {
		next, cmd, _ := m.press(id)
		return next, cmd
	}
	row, ok := m.rowAt(x, y)
	if !ok {
		return m, nil
	}
	now := time.Now()
	double := row == m.station.selected && now.Sub(m.station.lastClick) <= doubleClick
	if row != m.station.selected {
		m.station.confirming = false
	}
	m.station.selected, m.station.lastClick = row, now
	if double {
		m.station.lastClick = time.Time{}
		return m, m.act(actionOpen)
	}
	return m, nil
}

// panes is where the list and details frames are drawn.
type panes struct {
	listX, listWidth       int
	detailsX, detailsWidth int
	details                bool
}

// panes gives the list 3/5 of the width and the details the rest, when
// the details fit.
func (m Model) panes() panes {
	total := max(0, m.width-2*stationLeft)
	list := total * 3 / 5
	details := total - list - frameGap
	if details < detailsMin {
		return panes{listX: stationLeft, listWidth: total}
	}
	return panes{stationLeft, list, stationLeft + list + frameGap, details, true}
}

func (m Model) contentTop() int { return m.tabsTop() + tabsChrome }

// The list's rows start below its frame's top and its header.
func (m Model) stationTop() int { return m.contentTop() + 2 }

func (m Model) stationRows() int { return max(0, m.contentHeight()-3) }

// rowAt returns the track drawn at cell x, y.
func (m Model) rowAt(x, y int) (int, bool) {
	p := m.panes()
	row := y - m.stationTop() + m.station.offset
	if m.tab != tabStation || x <= p.listX || x >= p.listX+p.listWidth-1 || y < m.stationTop() ||
		y >= m.stationTop()+m.stationRows() || row >= len(m.station.tracks) {
		return 0, false
	}
	return row, true
}

// scrollStation keeps the selected row on screen.
func (m Model) scrollStation() Model {
	m.station.offset = keepVisible(m.station.selected, m.station.offset, m.stationRows(), len(m.station.tracks))
	return m
}

// stationView draws the list frame and, when it fits, the details
// frame: width by height cells.
func (m Model) stationView(width, height int) []string {
	s := m.station
	switch {
	case s.err != nil:
		return m.message(width, height, m.fg(theme.StateDanger).Render("Couldn't read the tracks: "+s.err.Error()))
	case len(s.tracks) == 0:
		return m.message(width, height, m.fg(theme.TextMuted).Render("No tracks yet."))
	}
	p := m.panes()
	list := m.frame("Tracks", theme.BorderDefault, m.list(p.listWidth-4, height-2), p.listWidth, height)
	var details []string
	if p.details {
		body, _ := m.details(p.detailsWidth - 4)
		if ft := m.fastTrackHeight(len(body)); ft > 0 {
			details = m.frame("Fast Track", theme.BorderAccent, m.fastTrack(p.detailsWidth-4), p.detailsWidth, ft)
		}
		details = append(details, m.frame("Details", theme.BorderDefault, body, p.detailsWidth, height-len(details))...)
	}
	margin := strings.Repeat(" ", stationLeft)
	lines := make([]string, len(list))
	for i := range list {
		line := margin + list[i]
		if p.details {
			line += strings.Repeat(" ", frameGap) + details[i]
		}
		lines[i] = pad(line, width)
	}
	return lines
}

// frame draws a rounded border in color, with title in its top line,
// around body: width by height cells with a space inside the sides.
func (m Model) frame(title string, color theme.Token, body []string, width, height int) []string {
	lines := make([]string, height)
	if width < 4 || height < 2 {
		for i := range lines {
			lines[i] = strings.Repeat(" ", max(0, width))
		}
		return lines
	}
	border := m.fg(color)
	title = cut(title, max(0, width-6))
	rest := width - 5 - lipgloss.Width(title)
	lines[0] = border.Render("╭─ " + title + " " + strings.Repeat("─", max(0, rest)) + "╮")
	for i := 1; i < height-1; i++ {
		inner := ""
		if i-1 < len(body) {
			inner = body[i-1]
		}
		lines[i] = border.Render("│") + " " + pad(inner, width-4) + " " + border.Render("│")
	}
	lines[height-1] = border.Render("╰" + strings.Repeat("─", width-2) + "╯")
	return lines
}

// list draws the header and the visible tracks, width by height cells.
func (m Model) list(width, height int) []string {
	s := m.station
	rows := make([][]string, len(s.tracks))
	for i, t := range s.tracks {
		rows[i] = cells(t)
	}
	return table{header: columns, rows: rows, right: costColumn, selected: s.selected, hover: s.hover, offset: s.offset}.draw(m, width, height)
}

// message centres one line in width by height cells.
func (m Model) message(width, height int, text string) []string {
	lines := strings.Split(lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, text), "\n")
	for i := range lines {
		lines[i] = pad(lines[i], width)
	}
	return lines[:min(len(lines), height)]
}
