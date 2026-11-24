package relaystation

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/relay"
	"github.com/bluegardenproject/tracks/internal/theme"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/ui/widget"
)

// FoldLines is how many lines of a long message show while it's folded.
const FoldLines = 3

// indent is where a message's lines start, after the selection mark.
const indent = "  "

// layout is the relay drawn as body lines: which message each line
// belongs to (-1 for none), and each message's first and last line.
type layout struct {
	lines       []string
	owner       []int
	heads, ends []int
}

func (m Model) fg(t theme.Token) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(m.palette.Color(t))
}

// layout draws every message: its head, then its body, folded unless
// it's open, a blank line between messages.
func (m Model) layout() layout {
	var l layout
	add := func(line string, owner int) {
		l.lines = append(l.lines, line)
		l.owner = append(l.owner, owner)
	}
	width := max(1, m.width-4-len(indent))
	for i, msg := range m.msgs {
		if i > 0 {
			add("", -1)
		}
		l.heads = append(l.heads, len(l.lines))
		mark := indent
		if i == m.selected {
			mark = m.fg(theme.TextAccent).Render("▌") + " "
		}
		if msg.Kind == relay.Notice {
			add(mark+m.fg(theme.TextFaint).Render(m.when(msg.At))+" "+m.fg(theme.TextMuted).Render(firstLine(clean(msg.Body))), i)
			l.ends = append(l.ends, len(l.lines)-1)
			continue
		}
		add(mark+m.head(msg), i)
		body := strings.Split(lipgloss.Wrap(clean(strings.TrimRight(msg.Body, "\n")), width, ""), "\n")
		shown := body
		if !m.open[msg.ID] && len(body) > FoldLines+1 {
			shown = body[:FoldLines]
		}
		for _, line := range shown {
			add(indent+m.fg(theme.TextDefault).Render(line), i)
		}
		switch {
		case len(shown) < len(body):
			add(indent+m.fg(theme.TextFaint).Render(fmt.Sprintf("… %d more lines · Enter opens", len(body)-len(shown))), i)
		case m.open[msg.ID] && len(body) > FoldLines+1:
			add(indent+m.fg(theme.TextFaint).Render("Enter folds"), i)
		}
		l.ends = append(l.ends, len(l.lines)-1)
	}
	return l
}

// head is a message's first line: when, from whom to whom, its kind,
// and whether it waits for its pointer.
func (m Model) head(msg relay.Message) string {
	to := "everyone"
	if msg.To != "" {
		to = "@" + msg.To
	}
	parts := []string{
		m.fg(theme.TextFaint).Render(m.when(msg.At)),
		m.fg(theme.TextDefault).Bold(true).Render("@"+msg.From) + m.fg(theme.TextMuted).Render(" → ") +
			m.fg(theme.TextDefault).Render(to),
		widget.Badge(m.palette, kindStatus(msg.Kind)),
	}
	if m.waiting[msg.ID] {
		parts = append(parts, widget.Badge(m.palette, notDelivered))
	}
	return strings.Join(parts, " ")
}

// notDelivered marks a message still waiting for its pointer.
var notDelivered = track.Status{ID: "not-delivered", Label: "not delivered", Badge: track.BadgeWarning}

// kindStatus is a message's kind as a badge: a question waits on
// someone, the rest inform.
func kindStatus(k relay.Kind) track.Status {
	badge := track.BadgeInfo
	if k == relay.Question {
		badge = track.BadgeWarning
	}
	return track.Status{ID: string(k), Label: string(k), Badge: badge}
}

// when is t as the Relay Station shows it: the time alone today, the
// day too before.
func (m Model) when(t time.Time) string {
	t, now := t.Local(), m.now().Local()
	if y, d := t.YearDay(), now.YearDay(); y == d && t.Year() == now.Year() {
		return t.Format("15:04")
	}
	return t.Format("Jan 2 15:04")
}

// clean is s without the control characters a message may carry, so a
// sender's escape sequences can't restyle or move the screen; tabs
// become spaces.
func clean(s string) string {
	s = strings.ReplaceAll(s, "\t", "    ")
	return strings.Map(func(r rune) rune {
		if r == '\n' || r >= 0x20 && r != 0x7f && (r < 0x80 || r > 0x9f) {
			return r
		}
		return -1
	}, s)
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}

// View implements tea.Model.
func (m Model) View() tea.View {
	v := tea.NewView(m.render())
	v.MouseMode = tea.MouseModeCellMotion
	v.AltScreen = true
	return v
}

func (m Model) render() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}
	var body []string
	switch {
	case m.err != "":
		body = []string{m.fg(theme.StateDangerText).Render("Couldn't read the relay: " + m.err)}
	case !m.loaded:
		body = []string{m.fg(theme.TextMuted).Render("Reading the relay…")}
	case len(m.msgs) == 0:
		body = []string{m.fg(theme.TextMuted).Render("No messages on this track's relay yet.")}
	default:
		lines := m.layout().lines
		body = lines[min(m.top, len(lines)):min(m.top+m.rows(), len(lines))]
	}
	title := "Relay Station"
	if n := len(m.msgs); n > 0 {
		title += fmt.Sprintf(" · %d messages", n)
	}
	lines := widget.Frame(m.palette, title, body, m.width, m.height-1, theme.OverlayBorderFocus)
	lines = append(lines, " "+widget.Hints(m.fg(theme.TextAccent), m.fg(theme.TextFaint), Keys))
	return strings.Join(lines, "\n")
}
