package themecreator

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
	"github.com/bluegardenproject/tracks/internal/v2/ui/style"
	"github.com/bluegardenproject/tracks/internal/v2/ui/widget"
)

const (
	nameWidth    = 20
	previewWidth = 10
	inputWidth   = len("[#rrggbb ]")
	columnGap    = 3
	buttonGap    = 2
	// Lines around the token list: title, rule and column names above;
	// a gap, the buttons and the status below.
	listTop     = 3
	chromeLines = listTop + 3
)

// Where a token row's parts start, in cells. Clicks are mapped with
// the same numbers.
const (
	darkInputX   = 1 + nameWidth + previewWidth + 1
	lightColumnX = darkInputX + inputWidth + columnGap
	lightInputX  = lightColumnX + previewWidth + 1
)

func (m Model) buttonsY() int { return listTop + m.listHeight() + 1 }

func (m Model) listHeight() int { return max(1, m.height-chromeLines) }

// scrollToFocus keeps the focused token's line in view.
func (m *Model) scrollToFocus() {
	if m.focus >= len(m.fields) {
		return
	}
	lines, tokenLine := layout()
	line := tokenLine[m.focus/2]
	h := m.listHeight()
	if line-1 < m.offset {
		m.offset = max(0, line-1) // the group name above, too
	}
	if line >= m.offset+h {
		m.offset = line - h + 1
	}
	m.offset = min(m.offset, max(0, lines-h))
}

// View renders the creator in width × height.
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}
	p := m.palette()
	fg := func(token theme.Token) lipgloss.Style { return lipgloss.NewStyle().Foreground(p.Color(token)) }

	title := " " + fg(theme.Accent).Bold(true).Render("Theme creator") + "  " +
		fg(theme.TextFaint).Render("Tab or ↑↓ move · Enter next · Esc cancel")
	rule := fg(theme.BorderDefault).Render(strings.Repeat("─", m.width))
	columns := " " + fg(theme.TextFaint).Render(pad("Token", nameWidth)+pad("Dark", previewWidth+10)+"Light")

	list := m.list(p)
	end := min(len(list), m.offset+m.listHeight())
	visible := list[m.offset:end]
	for len(visible) < m.listHeight() {
		visible = append(visible, "")
	}

	status := fg(theme.TextMuted)
	if m.statusErr {
		status = fg(theme.StateDanger)
	}
	return strings.Join(append([]string{title, rule, columns},
		append(visible, "", m.buttons(p), " "+status.Render(m.status))...), "\n")
}

func (m Model) list(p style.Palette) []string {
	draft := m.draft()
	dark, light := style.New(draft, true), style.New(draft, false)
	lines, tokenLine := layout()
	out := make([]string, lines)
	group := ""
	for i, token := range theme.All {
		line := tokenLine[i]
		if g := groupOf(token); g != group {
			group = g
			out[line-1] = " " + lipgloss.NewStyle().Foreground(p.Color(theme.TextDefault)).Bold(true).Render(g)
		}
		name := lipgloss.NewStyle().Foreground(p.Color(theme.TextMuted))
		if m.focus/2 == i && m.focus < len(m.fields) {
			name = lipgloss.NewStyle().Foreground(p.Color(theme.TextAccent))
		}
		out[line] = " " + name.Render(pad(string(token), nameWidth)) +
			preview(dark, token) + " " + m.input(p, 2*i) + strings.Repeat(" ", columnGap) +
			preview(light, token) + " " + m.input(p, 2*i+1)
	}
	return out
}

// preview shows token as it will look, in the variant of palette.
func preview(palette style.Palette, token theme.Token) string {
	c := palette.Color(token)
	on := lipgloss.NewStyle().Background(palette.Color(theme.BgBase))
	switch kindOf(token) {
	case kindText:
		return lipgloss.NewStyle().Background(palette.Color(backgroundOf(token))).Foreground(c).
			Render(pad(" Sample", previewWidth))
	case kindBackground:
		return lipgloss.NewStyle().Background(c).Render(strings.Repeat(" ", previewWidth))
	default:
		return lipgloss.NewStyle().Background(c).Render("   ") + on.Foreground(c).Render(pad(" Aa", previewWidth-3))
	}
}

func (m Model) input(p style.Palette, i int) string {
	f := m.fields[i]
	bracket := theme.BorderDefault
	switch {
	case !theme.ValidHex(value(f)):
		bracket = theme.StateDanger
	case i == m.focus:
		bracket = theme.BorderFocus
	}
	return widget.Input(p, f.input, len("#rrggbb")+1, bracket)
}

func (m Model) buttons(p style.Palette) string {
	var out []string
	for b, label := range buttonLabels {
		s := lipgloss.NewStyle().Padding(0, 2).Foreground(p.Color(theme.TextDefault)).Background(p.Color(theme.BgSurface))
		if m.focus == len(m.fields)+b {
			s = s.Foreground(p.Color(theme.TextInverse)).Background(p.Color(theme.Accent)).Bold(true)
		}
		out = append(out, s.Render(label))
	}
	return " " + strings.Join(out, strings.Repeat(" ", buttonGap))
}

// buttonWidth is a button's width: its label and padding.
func buttonWidth(b int) int { return lipgloss.Width(buttonLabels[b]) + 4 }

func pad(s string, width int) string {
	return s + strings.Repeat(" ", max(0, width-lipgloss.Width(s)))
}
