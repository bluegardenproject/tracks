package tracksview

import (
	"errors"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/v2/agents"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
	"github.com/bluegardenproject/tracks/internal/v2/ui/widget"
)

const (
	autoModeHint = "Without auto mode the agent asks before running commands or changing files, so tracks wait for you more often and work slows down. Ask, plan and doc tracks keep their own modes."
	// engineLabelWidth is the label column inside an engine's box.
	engineLabelWidth = 16
	modelFieldWidth  = 32
	modelInputWidth  = 34
)

// engineHit is where a control is drawn: from line of the tab's content,
// h lines tall, and from cell x of the window, w wide.
type engineHit struct {
	c             engineControl
	line, x, w, h int
}

// enginesContent is every line of the tab, before scrolling, and where
// its controls are, in focus order.
func (m Model) enginesContent() ([]string, []engineHit) {
	width := m.width
	w := max(0, width-2*stationLeft)
	indent := strings.Repeat(" ", stationLeft)
	var lines []string
	add := func(s string) { lines = append(lines, pad(indent+s, width)) }
	e := m.engines
	switch {
	case m.engineSource == nil:
		add(m.fg(theme.TextFaint).Render("Engines aren't available here."))
		return lines, nil
	case e.loadErr != nil:
		add(m.fg(theme.StateDangerText).Render(cut("Couldn't read the settings: "+e.loadErr.Error(), w)))
		return lines, nil
	case !e.loaded:
		add(m.fg(theme.TextFaint).Render("Reading the engines…"))
		return lines, nil
	}
	var hits []engineHit
	for i, en := range agents.All {
		if i > 0 {
			add("")
		}
		b := engineBody{m: m, id: en.ID}
		b.draw(en, max(0, w-4))
		color := theme.BorderDefault
		if e.editing && e.focus.engine == en.ID {
			color = theme.BorderFocus
		}
		top := len(lines) + 1
		for _, l := range m.frame(en.Name, color, b.lines, w, len(b.lines)+2) {
			add(l)
		}
		for _, h := range b.hits {
			h.line += top
			h.x += stationLeft + 2
			hits = append(hits, h)
		}
	}
	return lines, hits
}

func (m Model) engineHits() []engineHit {
	_, hits := m.enginesContent()
	return hits
}

// engineBody builds the inside of an engine's box.
type engineBody struct {
	m     Model
	id    string
	lines []string
	hits  []engineHit
}

func (b *engineBody) add(s string) { b.lines = append(b.lines, s) }

// hit marks a control on the next line added.
func (b *engineBody) hit(kind, index, x, w, h int) {
	b.hits = append(b.hits, engineHit{engineControl{b.id, kind, index}, len(b.lines), x, w, h})
}

// lit reports whether control kind is under the mouse or has focus.
func (b *engineBody) lit(kind, index int) bool {
	c := engineControl{b.id, kind, index}
	e := b.m.engines
	return e.hover == c || (e.editing && e.focus == c)
}

// buttons adds a row of buttons from cell x, before prefix.
func (b *engineBody) buttons(prefix string, x int, kinds []int, buttons ...widget.Button) {
	for i := range buttons {
		buttons[i].Hover = b.lit(kinds[i], 0)
	}
	row, starts := widget.ButtonRow(b.m.palette, buttons...)
	for i, start := range starts {
		b.hit(kinds[i], 0, x+start, buttons[i].Width(), 1)
	}
	b.add(prefix + row)
}

func (b *engineBody) label(s string) string {
	return b.m.fg(theme.TextMuted).Render(pad(s, engineLabelWidth))
}

func (b *engineBody) wrapped(token theme.Token, text string, width int) {
	for _, l := range strings.Split(b.m.fg(token).Width(max(1, width)).Render(text), "\n") {
		b.add(strings.Repeat(" ", engineLabelWidth) + l)
	}
}

func (b *engineBody) draw(en agents.Engine, width int) {
	m := b.m
	e := m.engines
	cur := e.settings.Get(en.ID)
	check := e.checks[en.ID]
	if cur == nil {
		switch {
		case check.checking:
			b.add("")
			b.add(lipgloss.PlaceHorizontal(width, lipgloss.Center, m.fg(theme.TextMuted).Render("Looking for "+en.Program+"…")))
			b.add("")
		case check.err != nil:
			b.add(m.fg(theme.StateDangerText).Render(cut(checkProblem(en, check.err), width)))
			b.add(cut(m.fg(theme.TextMuted).Render("Install it with ")+m.fg(theme.TextDefault).Render(en.Install), width))
			b.add("")
			b.buttons("", 0, []int{ctlCheck}, widget.NewButton("Check again", widget.ButtonAccent))
		default:
			label := "Add " + en.Name + " to Engines"
			w := lipgloss.Width(label) + 4
			x := max(0, (width-w)/2)
			b.add("")
			b.hit(ctlAdd, 0, x, w, 3)
			for _, l := range m.framedButton(label, b.lit(ctlAdd, 0)) {
				b.add(strings.Repeat(" ", x) + l)
			}
			b.add("")
		}
		return
	}

	b.status(en, check, width)
	b.add("")
	value, detail := "Default", en.Name+" chooses"
	if cur.Model != "" {
		value, detail = cur.Model, m.modelLabel(en, cur.Model)
	}
	fw := max(6, min(modelFieldWidth, width-engineLabelWidth-2))
	b.hit(ctlModel, 0, engineLabelWidth, fw+2, 1)
	b.add(b.label("Default model") + m.selectField(value, fw, b.lit(ctlModel, 0)) + m.fg(theme.TextFaint).Render(cut("  "+detail, max(0, width-engineLabelWidth-fw-2))))

	if !en.ListsModels {
		b.add("")
		b.models(en, cur.Models)
	}

	b.add("")
	b.mcp(en, width)

	b.add("")
	state, button := m.fg(theme.StateSuccessText).Bold(true).Render("On "), "Disable auto mode"
	if !cur.AutoMode() {
		state, button = m.fg(theme.TextMuted).Render("Off"), "Enable auto mode"
	}
	b.buttons(b.label("Auto mode")+state+"  ", engineLabelWidth+5, []int{ctlAuto}, widget.NewButton(button, widget.ButtonDefault))
	b.wrapped(theme.TextFaint, autoModeHint, width-engineLabelWidth)

	b.add("")
	if e.confirm == en.ID {
		b.add(m.fg(theme.TextDefault).Render(cut("Remove "+en.Name+" from Engines? Its model settings are removed too.", width)))
		b.buttons("", 0, []int{ctlConfirm, ctlCancel},
			widget.NewButton("Remove", widget.ButtonDanger), widget.NewButton("Cancel", widget.ButtonDefault))
		return
	}
	b.buttons("", 0, []int{ctlRemove}, widget.NewButton("Remove "+en.Name, widget.ButtonDanger))
}

// status is the badge, with where the CLI is or why it isn't there.
func (b *engineBody) status(en agents.Engine, check engineCheck, width int) {
	m := b.m
	badge := func(bg, fg theme.Token, text string) string {
		return lipgloss.NewStyle().Background(m.palette.Color(bg)).Foreground(m.palette.Color(fg)).Bold(true).Render(" " + text + " ")
	}
	switch {
	case check.checking:
		b.add(b.label("Status") + badge(theme.StateInfoBgAccent, theme.StateInfoTextAccent, "checking"))
	case check.err != nil:
		prefix := b.label("Status") + badge(theme.StateDangerBgAccent, theme.StateDangerTextAccent, "not found") + "  "
		b.buttons(prefix, lipgloss.Width(prefix), []int{ctlCheck}, widget.NewButton("Check again", widget.ButtonDefault))
		b.wrapped(theme.StateDangerText, checkProblem(en, check.err), width-engineLabelWidth)
		b.add(strings.Repeat(" ", engineLabelWidth) + cut(m.fg(theme.TextMuted).Render("Install it with ")+m.fg(theme.TextDefault).Render(en.Install), width-engineLabelWidth))
	default:
		where := check.found.Path
		if v := check.found.Version; v != "" {
			where += "  " + v
		}
		b.add(b.label("Status") + badge(theme.StateSuccessBgAccent, theme.StateSuccessTextAccent, "active") +
			m.fg(theme.TextFaint).Render(cut("  "+where, max(0, width-engineLabelWidth-8))))
	}
}

// models lists the built-in and added models, and the input to add one.
func (b *engineBody) models(en agents.Engine, added []string) {
	m := b.m
	idWidth := 0
	for _, model := range en.Models {
		idWidth = max(idWidth, lipgloss.Width(model.ID))
	}
	for _, id := range added {
		idWidth = max(idWidth, lipgloss.Width(id))
	}
	idWidth += 2
	for i, model := range en.Models {
		label := ""
		if i == 0 {
			label = "Models"
		}
		b.add(b.label(label) + m.fg(theme.TextDefault).Render(pad(model.ID, idWidth)) + m.fg(theme.TextFaint).Render(model.Label))
	}
	x := engineLabelWidth + idWidth
	for i, id := range added {
		remove := widget.NewButton("Remove", widget.ButtonDefault)
		remove.Hover = b.lit(ctlDropModel, i)
		b.hit(ctlDropModel, i, x, remove.Width(), 1)
		b.add(b.label("") + m.fg(theme.TextDefault).Render(pad(id, idWidth)) + remove.View(m.palette))
	}
	bracket := theme.BorderDefault
	if b.lit(ctlInput, 0) {
		bracket = theme.BorderFocus
	}
	b.hit(ctlInput, 0, engineLabelWidth, modelInputWidth+2, 1)
	b.buttons(b.label("")+widget.Input(m.palette, m.engines.input, modelInputWidth, bracket)+"  ",
		engineLabelWidth+modelInputWidth+4, []int{ctlAddModel}, widget.NewButton("Add model", widget.ButtonDefault))
}

// modelLabel is what en calls model, "" when it doesn't say.
func (m Model) modelLabel(en agents.Engine, model string) string {
	for _, known := range append(append([]agents.Model{}, en.Models...), m.engines.models[en.ID]...) {
		if known.ID == model {
			return known.Label
		}
	}
	return ""
}

// checkProblem says why en's CLI wasn't found.
func checkProblem(en agents.Engine, err error) string {
	if errors.Is(err, agents.ErrNotFound) {
		return "`" + en.Program + "` isn't on the PATH."
	}
	return err.Error()
}

// enginesView draws the tab, scrolled, width by height cells.
func (m Model) enginesView(width, height int) []string {
	lines, _ := m.enginesContent()
	offset := max(0, min(m.engines.offset, len(lines)-height))
	lines = lines[min(offset, len(lines)):min(len(lines), offset+height)]
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}
	return lines
}

// scrollEngines keeps the control with focus on screen.
func (m Model) scrollEngines() Model {
	lines, hits := m.enginesContent()
	e := &m.engines
	height := m.contentHeight()
	for _, h := range hits {
		if h.c == e.focus {
			if h.line < e.offset {
				e.offset = h.line
			}
			if h.line+h.h > e.offset+height {
				e.offset = h.line + h.h - height
			}
		}
	}
	e.offset = max(0, min(e.offset, len(lines)-height))
	return m
}

func (m Model) scrollEnginesBy(step int) Model {
	lines, _ := m.enginesContent()
	m.engines.offset = max(0, min(m.engines.offset+step, len(lines)-m.contentHeight()))
	return m
}

// engineAt returns the control at cell x, y of the window.
func (m Model) engineAt(x, y int) (engineControl, bool) {
	line := y - m.contentTop() + m.engines.offset
	if y < m.contentTop() || y >= m.contentTop()+m.contentHeight() {
		return engineControl{}, false
	}
	for _, h := range m.engineHits() {
		if line >= h.line && line < h.line+h.h && x >= h.x && x < h.x+h.w {
			return h.c, true
		}
	}
	return engineControl{}, false
}

// enginesClick focuses the control clicked, and presses it unless it's
// the input.
func (m Model) enginesClick(x, y int) (Model, tea.Cmd) {
	c, ok := m.engineAt(x, y)
	if !ok {
		return m, nil
	}
	m.engines.editing = true
	m = m.focusEngine(c)
	if c.kind == ctlInput {
		return m, nil
	}
	return m.pressEngine(c)
}

func (m Model) enginesHover(x, y int) Model {
	m.engines.hover, _ = m.engineAt(x, y)
	return m
}
