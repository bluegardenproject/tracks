package tracksview

import (
	"fmt"
	"strings"

	"github.com/bluegardenproject/tracks/internal/theme"
	"github.com/bluegardenproject/tracks/internal/ui/source"
	"github.com/bluegardenproject/tracks/internal/ui/widget"
)

const setupDesc = "Runs once in each worktree before its dev servers, such as pnpm install. " +
	"Work tracks run it when they start; other tracks when a server first needs it."

// formInput appends field's input under its label and description, and
// its error below it.
func (m Model) formInput(lines []string, hits []formHit, field formField, label, desc string, width int) ([]string, []formHit) {
	f := m.repos.form
	key := f.errKey(field)
	bracket := theme.BorderDefault
	switch {
	case f.errs[key] != "":
		bracket = theme.StateDangerText
	case m.repos.editing && f.focus == field:
		bracket = theme.BorderFocus
	}
	lines = append(lines, m.fg(theme.TextDefault).Render(label))
	lines = append(lines, m.wrap(theme.TextMuted, desc, width)...)
	hits = append(hits, formHit{field, len(lines), 0, width})
	lines = append(lines, widget.Input(m.palette, *f.input(field), m.inputWidth(), bracket))
	return m.formError(lines, key, width), hits
}

// formError appends the error under key, if there is one.
func (m Model) formError(lines []string, key string, width int) []string {
	if msg := m.repos.form.errs[key]; msg != "" {
		lines = append(lines, m.wrap(theme.StateDangerText, msg, width)...)
	}
	return lines
}

func (m Model) wrap(color theme.Token, text string, width int) []string {
	return strings.Split(m.fg(color).Width(max(1, width)).Render(text), "\n")
}

// setupAndServers appends the Setup and Dev servers sections that are
// shown, then the buttons that add the others.
func (m Model) setupAndServers(lines []string, hits []formHit, width int) ([]string, []formHit) {
	f := m.repos.form
	heading := func(title string) { lines = append(lines, m.fg(theme.TextDefault).Bold(true).Render(title)) }
	if f.setupOn {
		heading("Setup")
		lines, hits = m.formInput(lines, hits, fieldSetup, "Command", setupDesc, width)
		lines, hits = m.formButtons(lines, hits, []formButton{{fieldRemoveSetup, "Remove setup", widget.ButtonDefault}})
		lines = append(lines, "")
	}
	if len(f.servers) > 0 {
		heading("Dev servers")
		for i := range f.servers {
			lines, hits = m.serverSection(lines, hits, i, width)
			lines = append(lines, "")
		}
		lines, hits = m.formButtons(lines, hits, []formButton{{fieldAddServer, "Add server", widget.ButtonDefault}})
		lines = append(lines, "")
	}
	var add []formButton
	if !f.setupOn {
		add = append(add, formButton{fieldAddSetup, "Add setup", widget.ButtonDefault})
	}
	if len(f.servers) == 0 {
		add = append(add, formButton{fieldAddServer, "Add servers", widget.ButtonDefault})
	}
	if len(add) > 0 {
		lines, hits = m.formButtons(lines, hits, add)
		lines = append(lines, "")
	}
	return lines, hits
}

// serverSection appends dev server i's fields and its Remove button.
func (m Model) serverSection(lines []string, hits []formHit, i, width int) ([]string, []formHit) {
	s := m.repos.form.servers[i]
	title := fmt.Sprintf("Server %d", i+1)
	if name := s.value(serverName); name != "" {
		title += " · " + name
	}
	lines = append(lines, m.fg(theme.TextMuted).Render(cut(title, width)))
	for _, k := range s.fields() {
		field := serverField(i, k)
		switch k {
		case serverMode:
			lines, hits = m.portModeRow(lines, hits, i, width)
		case serverRemove:
			lines, hits = m.formButtons(lines, hits, []formButton{{field, "Remove server", widget.ButtonDefault}})
		default:
			in := serverInputs[k]
			lines, hits = m.formInput(lines, hits, field, in.label, in.desc, width)
		}
		if k != serverRemove {
			lines = append(lines, "")
		}
	}
	return lines, hits
}

// portModeRow appends dev server i's port mode as a row of choices,
// the chosen one marked. Space or a click moves to the next.
func (m Model) portModeRow(lines []string, hits []formHit, i, width int) ([]string, []formHit) {
	f := m.repos.form
	s := f.servers[i]
	field := serverField(i, serverMode)
	desc := ""
	mark := theme.BorderDefault
	if (m.repos.editing && f.focus == field) || m.repos.hoverField == field {
		mark = theme.BorderFocus
	}
	var choices []string
	for _, pm := range portModes {
		box := "( )"
		if pm.mode == s.mode {
			box, desc = "(•)", pm.desc
		}
		choices = append(choices, m.fg(mark).Render(box)+" "+m.fg(theme.TextDefault).Render(pm.mode))
	}
	lines = append(lines, m.fg(theme.TextDefault).Render("Port"))
	lines = append(lines, m.wrap(theme.TextMuted, desc, width)...)
	hits = append(hits, formHit{field, len(lines), 0, width})
	lines = append(lines, strings.Join(choices, "   "))
	if s.mode != source.PortFixed {
		lines = m.formError(lines, f.errKey(field), width)
	}
	return lines, hits
}

// scrollForm scrolls the form so the focused field shows.
func (m Model) scrollForm() Model {
	rp := m.repoPanes()
	visible := m.contentHeight() - 2
	if !rp.form || visible <= 0 {
		return m
	}
	body, hits := m.repoForm(rp.formW-4, visible)
	r := &m.repos
	for _, h := range hits {
		if h.field != r.form.focus || !r.editing {
			continue
		}
		if h.row < r.formOffset {
			r.formOffset = max(0, h.row-3) // keep the label and description in view
		}
		if h.row >= r.formOffset+visible {
			r.formOffset = h.row - visible + 2
		}
		break
	}
	r.formOffset = max(0, min(r.formOffset, len(body)-visible))
	return m
}

// onRepoForm reports whether column x is over the form.
func (m Model) onRepoForm(x int) bool {
	rp := m.repoPanes()
	return rp.form && x >= rp.formX && x < rp.formX+rp.formW
}

// scrollFormBy scrolls the form by delta lines.
func (m Model) scrollFormBy(delta int) Model {
	m.repos.formOffset = max(0, m.repos.formOffset+delta)
	visible := m.contentHeight() - 2
	rp := m.repoPanes()
	if rp.form && visible > 0 {
		body, _ := m.repoForm(rp.formW-4, visible)
		m.repos.formOffset = min(m.repos.formOffset, max(0, len(body)-visible))
	}
	return m
}
