package tracksview

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
	"github.com/bluegardenproject/tracks/internal/v2/ui/widget"
)

// newButton adds a repo, and a Fast Track on the Settings tab.
var newButton = widget.NewButton("+ New", widget.ButtonDefault)

var repoColumns = []string{"Name", "Base"}

// The list takes a quarter of the width, unframed; the form's frame
// the rest. Narrower windows show whichever has focus.
const (
	repoListMin = 16
	repoGap     = 2
)

// repoPanes is where the list and the form are.
type repoPanes struct {
	list, form   bool
	listX, listW int
	formX, formW int
}

func (m Model) repoPanes() repoPanes {
	total := max(0, m.width-2*stationLeft)
	list := total / 4
	form := total - list - repoGap
	switch {
	case list >= repoListMin && form >= detailsMin:
		return repoPanes{true, true, stationLeft, list, stationLeft + list + repoGap, form}
	case m.repos.editing:
		return repoPanes{form: true, formX: stationLeft, formW: total}
	}
	return repoPanes{list: true, listX: stationLeft, listW: total}
}

// inputWidth is the width inside an input's brackets, which span the
// form's frame.
func (m Model) inputWidth() int {
	rp := m.repoPanes()
	w := rp.formW
	if !rp.form {
		w = max(0, m.width-2*stationLeft)
	}
	return max(4, w-4-2)
}

// The New button is on the list's first line, the rows below a blank
// line and the header.
func (m Model) repoRowsTop() int { return m.contentTop() + 3 }

func (m Model) repoRows() int { return max(0, m.contentHeight()-3) }

func (m Model) reposView(width, height int) []string {
	switch {
	case m.reposErr != nil:
		return m.message(width, height, m.fg(theme.StateDangerText).Render("Couldn't open the database: "+m.reposErr.Error()))
	case m.repoSource == nil:
		return m.message(width, height, m.fg(theme.TextMuted).Render("No database."))
	case m.repos.err != nil:
		return m.message(width, height, m.fg(theme.StateDangerText).Render("Couldn't read the repositories: "+m.repos.err.Error()))
	}
	rp := m.repoPanes()
	var left, right []string
	if rp.list {
		left = m.repoList(rp.listW, height)
		for len(left) < height {
			left = append(left, "")
		}
		for i := range left {
			left[i] = pad(left[i], rp.listW)
		}
	}
	if rp.form {
		color, title := theme.BorderDefault, "Details"
		if m.repos.editing {
			color = theme.BorderAccent
		}
		if m.repos.form.id == 0 {
			title = "New repository"
		}
		body, _ := m.repoForm(rp.formW-4, height-2)
		right = m.frame(title, color, body, rp.formW, height)
	}
	margin := strings.Repeat(" ", stationLeft)
	lines := make([]string, height)
	for i := range lines {
		line := margin
		if left != nil {
			line += left[i]
		}
		if left != nil && right != nil {
			line += strings.Repeat(" ", repoGap)
		}
		if right != nil {
			line += right[i]
		}
		lines[i] = pad(line, width)
	}
	return lines
}

func (m Model) repoList(width, height int) []string {
	r := m.repos
	button := newButton
	button.Hover = r.hoverNew
	view := button.View(m.palette)
	if r.selected == -1 {
		view = lipgloss.NewStyle().Background(m.palette.Color(theme.TableBgSelected)).
			Foreground(m.palette.Color(theme.TableTextDefault)).Bold(true).Render(" " + button.Label + " ")
	}
	lines := []string{view, ""}
	rows := make([][]string, len(r.entries))
	for i, e := range r.entries {
		rows[i] = []string{e.Name, e.BaseBranch}
	}
	t := table{header: repoColumns, rows: rows, right: -1, selected: r.selected, hover: r.hover, offset: r.offset}
	lines = append(lines, t.draw(m, width, height-2)...)
	if len(r.entries) == 0 {
		lines = append(lines, m.fg(theme.TextFaint).Render("No repositories yet. Add one with New."))
	}
	return lines
}

// formHit is where a form field is drawn, relative to the form's body.
type formHit struct {
	field       formField
	row, col, w int
}

// repoForm draws the form, width by height cells, and reports where its
// fields are: each input under its title, then the options. Delete sits
// on the last line.
func (m Model) repoForm(width, height int) ([]string, []formHit) {
	f := m.repos.form
	focused := func(field formField) bool { return m.repos.editing && f.focus == field }
	var lines []string
	var hits []formHit
	at := func(field formField, col, w int) { hits = append(hits, formHit{field, len(lines), col, w}) }

	for i, in := range inputs {
		if i > 0 {
			lines = append(lines, "")
		}
		bracket := theme.BorderDefault
		switch {
		case f.errs[in.key] != "":
			bracket = theme.StateDangerText
		case focused(in.field):
			bracket = theme.BorderFocus
		}
		lines = append(lines, m.fg(theme.TextDefault).Render(in.label))
		lines = append(lines, strings.Split(m.fg(theme.TextMuted).Width(max(1, width)).Render(in.desc), "\n")...)
		at(in.field, 0, width)
		lines = append(lines, widget.Input(m.palette, f.inputs[in.field], m.inputWidth(), bracket))
		if msg := f.errs[in.key]; msg != "" {
			wrapped := m.fg(theme.StateDangerText).Width(max(1, width)).Render(msg)
			lines = append(lines, strings.Split(wrapped, "\n")...)
		}
		if in.field == fieldPath && f.remote != "" {
			lines = append(lines, " "+m.fg(theme.TextFaint).Render(shorten(f.remote, width-1)))
		}
	}

	lines = append(lines, "", m.fg(theme.TextDefault).Bold(true).Render("Options"))
	box := "[ ]"
	if f.drafts {
		box = "[x]"
	}
	boxColor := theme.BorderDefault
	if focused(fieldDrafts) {
		boxColor = theme.BorderFocus
	}
	at(fieldDrafts, 0, width)
	lines = append(lines, m.fg(boxColor).Render(box)+" "+m.fg(theme.TextDefault).Render("Open pull requests as drafts"), "")

	if f.id != 0 {
		lines = append(lines, m.fg(theme.TextDefault).Bold(true).Render("Active tracks"))
		if len(f.tracks) == 0 {
			lines = append(lines, m.fg(theme.TextMuted).Render("None"))
		} else {
			count := fmt.Sprintf("%d  ", len(f.tracks))
			lines = append(lines,
				m.fg(theme.TextDefault).Render(count)+m.fg(theme.TextMuted).Render(cut(strings.Join(f.tracks, ", "), width-len(count))))
			note := m.fg(theme.TextFaint).Width(max(1, width)).Render("Name and path can't change while tracks run here.")
			lines = append(lines, strings.Split(note, "\n")...)
		}
		lines = append(lines, "")
	}

	switch {
	case m.repos.leaving != nil:
		lines = append(lines, m.fg(theme.StateWarningText).Render("Unsaved changes."))
		lines, hits = m.formButtons(lines, hits, []formButton{
			{fieldPromptSave, "Save", widget.ButtonDefault}, {fieldPromptDiscard, "Discard", widget.ButtonDefault}, {fieldPromptCancel, "Cancel", widget.ButtonDefault}})
	case f.dirty():
		lines, hits = m.formButtons(lines, hits, []formButton{{fieldSave, "Save", widget.ButtonDefault}})
	}

	if f.id != 0 {
		var bottom []string
		var bottomHits []formHit
		buttons := []formButton{{fieldDelete, "Delete", widget.ButtonDanger}}
		if f.confirming {
			question := m.fg(theme.StateWarningText).Width(max(1, width)).Render("Delete " + f.original.Name + "? Past tracks keep their history.")
			bottom = strings.Split(question, "\n")
			buttons = []formButton{{fieldConfirmDelete, "Delete repository", widget.ButtonDanger}, {fieldCancelDelete, "Cancel", widget.ButtonDefault}}
		}
		bottom, bottomHits = m.formButtons(bottom, nil, buttons)
		start := max(len(lines), height-len(bottom))
		for len(lines) < start {
			lines = append(lines, "")
		}
		for _, h := range bottomHits {
			h.row += start
			hits = append(hits, h)
		}
		lines = append(lines, bottom...)
	}
	return lines, hits
}

type formButton struct {
	field formField
	label string
	kind  widget.ButtonKind
}

// formButtons appends a row of buttons to lines. The focused one is
// drawn in its hover shade.
func (m Model) formButtons(lines []string, hits []formHit, buttons []formButton) ([]string, []formHit) {
	row := make([]widget.Button, len(buttons))
	for i, b := range buttons {
		row[i] = widget.NewButton(b.label, b.kind)
		row[i].Hover = (m.repos.editing && m.repos.form.focus == b.field) || m.repos.hoverField == b.field
	}
	line, starts := widget.ButtonRow(m.palette, row...)
	for i, b := range buttons {
		hits = append(hits, formHit{b.field, len(lines), starts[i], row[i].Width()})
	}
	return append(lines, line), hits
}

// repoRowAt returns the repo drawn at cell x, y.
func (m Model) repoRowAt(x, y int) (int, bool) {
	rp := m.repoPanes()
	row := y - m.repoRowsTop() + m.repos.offset
	if !rp.list || x < rp.listX || x >= rp.listX+rp.listW || y < m.repoRowsTop() ||
		y >= m.repoRowsTop()+m.repoRows() || row >= len(m.repos.entries) {
		return 0, false
	}
	return row, true
}

// onNewRepo reports whether cell x, y is on the list's New button.
func (m Model) onNewRepo(x, y int) bool {
	rp := m.repoPanes()
	return rp.list && y == m.contentTop() && x >= rp.listX && x < rp.listX+newButton.Width()
}

// repoFieldAt returns the form field drawn at cell x, y.
func (m Model) repoFieldAt(x, y int) (formField, bool) {
	rp := m.repoPanes()
	if !rp.form {
		return 0, false
	}
	top := m.contentTop() + 1
	_, hits := m.repoForm(rp.formW-4, m.contentHeight()-2)
	for _, h := range hits {
		if y == top+h.row && x >= rp.formX+2+h.col && x < rp.formX+2+h.col+h.w {
			return h.field, true
		}
	}
	return 0, false
}

func (m Model) repoClick(x, y int) (Model, tea.Cmd) {
	m.repos.notice = notice{}
	if m.onNewRepo(x, y) {
		return m.request(leave{kind: leaveNew})
	}
	if row, ok := m.repoRowAt(x, y); ok {
		if row == m.repos.selected && !m.repos.editing {
			return m, nil
		}
		return m.request(leave{kind: leaveRow, row: row})
	}
	if field, ok := m.repoFieldAt(x, y); ok {
		prompt := field >= fieldPromptSave
		if (m.repos.leaving != nil) != prompt {
			return m, nil
		}
		m.repos.editing = true
		if field <= fieldBase {
			return m, m.repos.form.setFocus(field)
		}
		m.repos.form.focus = field
		for i := range m.repos.form.inputs {
			m.repos.form.inputs[i].Blur()
		}
		return m.pressField(field)
	}
	return m, nil
}
