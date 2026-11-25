package tracksview

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/settings"
	"github.com/bluegardenproject/tracks/internal/theme"
)

// githubState is Settings → GitHub: what Tracks does when GitHub reports
// something on a side track's PR.
type githubState struct {
	// github is as read: a value this build doesn't know stays until its
	// row is changed, and shows as the default.
	github  settings.GitHub
	loadErr error
	// saving is a save on its way; pending is another one due after it.
	saving, pending bool
	focus           int
	hover           githubHit
}

// githubHit is a choice on the section: its row and its policy, -1 for
// none.
type githubHit struct{ row, choice int }

var githubRows = []struct{ event, label string }{
	{settings.EventConflicts, "Conflicts with its base"},
	{settings.EventCIFailed, "CI failed"},
	{settings.EventReviewThreads, "New review threads"},
}

// githubChoices are a row's choices, in order, with what the notice says
// when one is chosen.
var githubChoices = []struct {
	policy        settings.Policy
	label, notice string
}{
	{settings.PolicyAuto, "Handle automatically", "handled automatically"},
	{settings.PolicyMain, "Main track decides", "the main track decides"},
	{settings.PolicyAsk, "Ask me", "Tracks asks you"},
}

const (
	githubIntro = "What Tracks does when GitHub reports something on a side track's PR. The main track always hears of it."
	githubGap   = 2 // cells between two choices
)

var githubHelp = []string{
	"Handle automatically: Tracks tells the side track to handle it, and the main track as information.",
	"Main track decides: the main track's agent chooses, and may ask you.",
	"Ask me: the main track asks you.",
	"",
	"Merged and closed PRs always go to the main track.",
}

type (
	githubMsg struct {
		github settings.GitHub
		err    error
	}
	githubSavedMsg struct{ err error }
)

func (m Model) loadGitHub() tea.Cmd {
	if m.githubSource == nil {
		return nil
	}
	src := m.githubSource
	return func() tea.Msg {
		g, err := src.Load()
		return githubMsg{g, err}
	}
}

func (m Model) setGitHub(msg githubMsg) Model {
	g := &m.settings.github
	if !g.saving && !g.pending {
		g.github = msg.github
	}
	g.loadErr = msg.err
	return m
}

func (m Model) saveGitHub() (Model, tea.Cmd) {
	g := &m.settings.github
	if m.githubSource == nil {
		return m, nil
	}
	if g.saving {
		g.pending = true
		return m, nil
	}
	g.saving = true
	src, gh := m.githubSource, g.github
	return m, func() tea.Msg { return githubSavedMsg{src.Save(gh)} }
}

func (m Model) githubSaved(msg githubSavedMsg) (Model, tea.Cmd) {
	g := &m.settings.github
	g.saving = false
	if msg.err != nil {
		m.settings.notice = notice{text: "Couldn't save the GitHub settings: " + msg.err.Error(), err: true}
	}
	if g.pending {
		g.pending = false
		return m.saveGitHub()
	}
	return m, nil
}

// githubChoice is the choice row shows.
func (g githubState) githubChoice(row int) int {
	p := g.github.Policy(githubRows[row].event)
	for i, c := range githubChoices {
		if c.policy == p {
			return i
		}
	}
	return 0
}

// setGitHubPolicy gives row the policy of choice and saves it; choosing
// what it shows already changes nothing.
func (m Model) setGitHubPolicy(row, choice int) (Model, tea.Cmd) {
	g := &m.settings.github
	if choice == g.githubChoice(row) {
		return m, nil
	}
	c := githubChoices[choice]
	switch githubRows[row].event {
	case settings.EventConflicts:
		g.github.Conflicts = c.policy
	case settings.EventCIFailed:
		g.github.CIFailed = c.policy
	case settings.EventReviewThreads:
		g.github.ReviewThreads = c.policy
	}
	m.settings.notice = notice{text: githubRows[row].label + ": " + c.notice + "."}
	return m.saveGitHub()
}

// githubKey handles a key while GitHub has focus.
func (m Model) githubKey(key string) (Model, tea.Cmd) {
	g := &m.settings.github
	choice := g.githubChoice(g.focus)
	switch key {
	case "up", "k", "shift+tab":
		g.focus = max(0, g.focus-1)
	case "down", "j", "tab":
		g.focus = min(len(githubRows)-1, g.focus+1)
	case "left", "h":
		return m.setGitHubPolicy(g.focus, max(0, choice-1))
	case "right", "l":
		return m.setGitHubPolicy(g.focus, min(len(githubChoices)-1, choice+1))
	case "enter", "space":
		return m.setGitHubPolicy(g.focus, (choice+1)%len(githubChoices))
	}
	return m, nil
}

// githubLaid is a choice where githubView drew it: its line and its
// first and last cells, the last excluded.
type githubLaid struct {
	githubHit
	line, x0, x1 int
}

// githubView is the GitHub section, and where its choices are, counted
// from its top. A row's choices wrap onto more lines when they don't fit.
func (m Model) githubView(width int) (lines []string, laid []githubLaid) {
	s := m.settings
	g := s.github
	wrap := func(c theme.Token, text string) []string {
		return strings.Split(m.fg(c).Width(max(1, width)).Render(text), "\n")
	}
	lines = wrap(theme.TextMuted, githubIntro)
	for row, r := range githubRows {
		lines = append(lines, "", m.fg(theme.TextDefault).Render(cut(r.label, width)))
		selected := g.githubChoice(row)
		line, x := "  ", 2
		for i, c := range githubChoices {
			label := cut(c.label, max(0, width-6))
			w := 4 + lipgloss.Width(label)
			if x > 2 && x+githubGap+w > width {
				lines = append(lines, line)
				line, x = "  ", 2
			}
			if x > 2 {
				line += strings.Repeat(" ", githubGap)
				x += githubGap
			}
			mark, color := "( )", theme.BorderDefault
			if i == selected {
				mark = "(•)"
			}
			if g.hover == (githubHit{row, i}) || s.editing && g.focus == row && i == selected {
				color = theme.BorderFocus
			}
			laid = append(laid, githubLaid{githubHit{row, i}, len(lines), x, x + w})
			line += m.fg(color).Render(mark) + " " + m.fg(theme.TextDefault).Render(label)
			x += w
		}
		lines = append(lines, line)
	}
	lines = append(lines, "")
	for _, h := range githubHelp {
		if h == "" {
			lines = append(lines, "")
			continue
		}
		lines = append(lines, wrap(theme.TextMuted, h)...)
	}
	if g.loadErr != nil {
		lines = append(lines, "", m.fg(theme.StateDangerText).Render(cut("Couldn't read the settings: "+g.loadErr.Error(), width)))
	}
	return lines, laid
}

// githubAt is the choice at x, y relative to the section's inside.
func (m Model) githubAt(bx, by int) githubHit {
	_, laid := m.githubView(m.sectionWidth())
	for _, l := range laid {
		if by == l.line && bx >= l.x0 && bx < l.x1 {
			return l.githubHit
		}
	}
	return githubHit{-1, -1}
}

func (m Model) githubClick(bx, by int) (Model, tea.Cmd) {
	hit := m.githubAt(bx, by)
	if hit.row < 0 {
		return m, nil
	}
	m.settings.github.focus = hit.row
	return m.setGitHubPolicy(hit.row, hit.choice)
}
