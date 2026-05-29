// Package dashboard renders the live track list as a bubbletea TUI.
//
// It polls the daemon every second for tracks and pending permission
// prompts. The user can:
//
//   - press enter to switch the tmux client to the highlighted
//     track's window;
//   - press y / n to approve or deny a pending permission prompt;
//   - press q to quit.
//
// The dashboard is intentionally read-only on the state file —
// every mutation goes through the daemon socket.
package dashboard

import (
	"fmt"
	"strings"
	"time"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/daemon"
	"github.com/bluegardenproject/tracks/internal/state"
	"github.com/bluegardenproject/tracks/internal/tmux"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// pollInterval is the cadence at which the dashboard re-reads
// daemon state. One second is fast enough to feel live but slow
// enough to keep CPU near-idle.
const pollInterval = 1 * time.Second

// styles holds all lipgloss styles. Centralized so a future theme
// switch is a single edit.
type styles struct {
	header        lipgloss.Style
	row           lipgloss.Style
	rowActive     lipgloss.Style
	status        map[state.Status]lipgloss.Style
	prompt        lipgloss.Style
	dim           lipgloss.Style
	pr            lipgloss.Style
	snippetText   lipgloss.Style
	snippetBorder lipgloss.Style
}

func defaultStyles() styles {
	return styles{
		header:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")),
		row:       lipgloss.NewStyle(),
		rowActive: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Background(lipgloss.Color("236")),
		status: map[state.Status]lipgloss.Style{
			state.StatusPending: lipgloss.NewStyle().Foreground(lipgloss.Color("11")),
			state.StatusRunning: lipgloss.NewStyle().Foreground(lipgloss.Color("10")),
			// Hot pink — a waiting track is blocking the developer
			// and should jump out of the table.
			state.StatusWaiting: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("207")),
			state.StatusDone:    lipgloss.NewStyle().Foreground(lipgloss.Color("8")),
			state.StatusErrored: lipgloss.NewStyle().Foreground(lipgloss.Color("9")),
		},
		prompt:        lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Background(lipgloss.Color("3")).Padding(0, 1),
		dim:           lipgloss.NewStyle().Foreground(lipgloss.Color("8")),
		pr:            lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Underline(true),
		snippetText:   lipgloss.NewStyle().Foreground(lipgloss.Color("250")),
		snippetBorder: lipgloss.NewStyle().Foreground(lipgloss.Color("207")),
	}
}

// Run launches the dashboard in the current terminal. Blocks until
// the user quits.
func Run(cfg config.Config) error {
	m := newModel(cfg)
	_, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

// model is the bubbletea Model.
type model struct {
	cfg      config.Config
	client   *daemon.Client
	tmux     *tmux.Client
	styles   styles
	tracks   []state.Track
	prompts  []daemon.PendingPrompt
	cursor   int // selected row in the tracks table
	width    int
	height   int
	err      error
	lastPoll time.Time
}

func newModel(cfg config.Config) *model {
	return &model{
		cfg:    cfg,
		client: daemon.NewClient(cfg),
		tmux:   tmux.New(),
		styles: defaultStyles(),
	}
}

// tickMsg is sent on every pollInterval to trigger a refresh.
type tickMsg time.Time

// pollResult is what the refresh goroutine returns.
type pollResult struct {
	tracks  []state.Track
	prompts []daemon.PendingPrompt
	err     error
}

func (m *model) Init() tea.Cmd {
	return tea.Batch(m.poll(), tickEvery())
}

func tickEvery() tea.Cmd {
	return tea.Tick(pollInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// poll fetches latest tracks + prompts. Errors are surfaced into
// model.err so the UI still renders something useful.
func (m *model) poll() tea.Cmd {
	cfg := m.cfg
	return func() tea.Msg {
		cl := daemon.NewClient(cfg)
		tracks, err := cl.Ls()
		if err != nil {
			return pollResult{err: err}
		}
		prompts, err := cl.PendingPrompts()
		if err != nil {
			// Pending prompts is best-effort; don't fail the whole
			// poll on its error.
			return pollResult{tracks: tracks}
		}
		return pollResult{tracks: tracks, prompts: prompts}
	}
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.tracks)-1 {
				m.cursor++
			}
		case "enter":
			if len(m.tracks) > 0 {
				t := m.tracks[m.cursor]
				_ = m.attachTrack(t.ID)
			}
		case "y", "Y":
			if len(m.prompts) > 0 {
				_ = m.client.AnswerPrompt(m.prompts[0].ID, true)
				return m, m.poll()
			}
		case "n", "N":
			if len(m.prompts) > 0 {
				_ = m.client.AnswerPrompt(m.prompts[0].ID, false)
				return m, m.poll()
			}
		case "x":
			// Forget the highlighted track (must be terminal).
			if len(m.tracks) > 0 {
				t := m.tracks[m.cursor]
				if t.Status.IsTerminal() {
					_ = m.client.Forget(t.ID)
					return m, m.poll()
				}
			}
		case "X":
			// Prune every completed track.
			_, _ = m.client.PruneCompleted()
			return m, m.poll()
		case "d":
			// Graceful end of the highlighted track (must be active).
			if len(m.tracks) > 0 {
				t := m.tracks[m.cursor]
				if !t.Status.IsTerminal() {
					_ = m.client.Done(t.ID)
					m.closeTrackWindow(t.ID)
					return m, m.poll()
				}
			}
		case "K":
			// Force kill the highlighted track (must be active).
			// Capital K to distinguish from lowercase k (cursor-up
			// vim convention) and to make accidental kills harder.
			if len(m.tracks) > 0 {
				t := m.tracks[m.cursor]
				if !t.Status.IsTerminal() {
					_ = m.client.Kill(t.ID)
					m.closeTrackWindow(t.ID)
					return m, m.poll()
				}
			}
		case "r":
			return m, m.poll()
		}
	case tickMsg:
		return m, tea.Batch(m.poll(), tickEvery())
	case pollResult:
		m.lastPoll = time.Now()
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.err = nil
			m.tracks = msg.tracks
			m.prompts = msg.prompts
			if m.cursor >= len(m.tracks) {
				m.cursor = max(0, len(m.tracks)-1)
			}
		}
	}
	return m, nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// closeTrackWindow removes the per-track tmux window after the
// track has been ended. Best-effort — failure is silent because the
// daemon side has already mutated state.
func (m *model) closeTrackWindow(trackID string) {
	window := "t-" + trackID[len(trackID)-min(6, len(trackID)):]
	if exists, _ := m.tmux.HasWindow(m.cfg.Tmux.SessionName, window); !exists {
		return
	}
	_ = m.tmux.KillWindow(m.cfg.Tmux.SessionName, window)
}

// attachTrack ensures the per-track window exists and switches to it.
func (m *model) attachTrack(trackID string) error {
	window := "t-" + trackID[len(trackID)-min(6, len(trackID)):]
	session := m.cfg.Tmux.SessionName
	exists, _ := m.tmux.HasWindow(session, window)
	if !exists {
		// We let the user attach via the `tracks attach` CLI rather
		// than duplicate the window-recreation logic here. Showing
		// an error is honest.
		return fmt.Errorf("window %s missing — run `tracks attach %s` from a shell", window, trackID)
	}
	return m.tmux.SelectWindow(session, window)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// minSideBySideWidth is the smallest total terminal width at which
// the dashboard renders the left/right split. Below this we fall
// back to a stacked single-column layout so narrow terminals still
// look usable.
const minSideBySideWidth = 120

func (m *model) View() string {
	if m.width >= minSideBySideWidth {
		left := m.renderLeft(m.leftWidth())
		right := m.renderRight(m.rightWidth())
		return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	}
	return m.renderLeft(m.width) + "\n" + m.renderRight(m.width)
}

// leftWidth / rightWidth split the screen 55/45 with a small gutter.
func (m *model) leftWidth() int {
	if m.width <= 0 {
		return 80
	}
	w := m.width*55/100 - 1
	if w < 60 {
		w = 60
	}
	return w
}

func (m *model) rightWidth() int {
	if m.width <= 0 {
		return 60
	}
	w := m.width - m.leftWidth() - 2
	if w < 40 {
		w = 40
	}
	return w
}

// renderLeft draws the table + footer. Width clamps row formatting
// so we don't overflow into the right panel.
func (m *model) renderLeft(width int) string {
	var b strings.Builder

	b.WriteString(m.styles.header.Render("tracks — dashboard"))
	b.WriteString("  ")
	b.WriteString(m.styles.dim.Render(fmt.Sprintf("(%d tracks, %d pending prompts)", len(m.tracks), len(m.prompts))))
	b.WriteString("\n\n")

	for _, p := range m.prompts {
		b.WriteString(m.styles.prompt.Render(fmt.Sprintf("APPROVAL  %s wants %s — %s   [y=allow / n=deny]",
			shortID(p.TrackID), p.Tool, p.Detail)))
		b.WriteString("\n")
	}
	if len(m.prompts) > 0 {
		b.WriteString("\n")
	}

	if m.err != nil {
		b.WriteString(m.styles.dim.Render("daemon unreachable: ") + m.err.Error() + "\n")
	} else if len(m.tracks) == 0 {
		b.WriteString(m.styles.dim.Render("no tracks yet — run `tracks new`\n"))
	} else {
		b.WriteString(m.styles.header.Render(fmt.Sprintf("  %-15s  %-22s  %-10s  %-18s  %s",
			"ID", "BRANCH", "STATUS", "REPOS", "PR")))
		b.WriteString("\n")
		for i, t := range m.tracks {
			line := fmt.Sprintf("  %-15s  %-22s  %s  %-18s  %s",
				shortID(t.ID),
				truncate(t.Branch, 22),
				m.styles.status[t.Status].Render(padRight(string(t.Status), 10)),
				truncate(joinRepos(t.Repos), 18),
				m.styles.pr.Render(t.PRURL),
			)
			if i == m.cursor {
				b.WriteString(m.styles.rowActive.Render(line))
			} else {
				b.WriteString(line)
			}
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(m.styles.dim.Render("↑/↓ select   enter attach   d end   K kill   x forget   X clear completed   y/n approve"))
	b.WriteString("\n")
	b.WriteString(m.styles.dim.Render("r refresh   q quit   (open menu from any window with <prefix>+t)"))
	return lipgloss.NewStyle().Width(width).Render(b.String())
}

// renderRight draws the detail panel for the selected track.
func (m *model) renderRight(width int) string {
	if len(m.tracks) == 0 || m.cursor >= len(m.tracks) {
		hint := m.styles.dim.Render("select a track on the left to see details")
		return lipgloss.NewStyle().Width(width).Padding(1, 2).Render(hint)
	}
	t := m.tracks[m.cursor]

	var b strings.Builder
	b.WriteString(m.styles.header.Render("Track details"))
	b.WriteString("\n\n")

	field := func(k, v string) {
		if v == "" {
			return
		}
		b.WriteString(m.styles.dim.Render(k + ":  "))
		b.WriteString(v)
		b.WriteString("\n")
	}
	field("id", t.ID)
	field("branch", t.Branch)
	field("repos", joinRepos(t.Repos))
	b.WriteString(m.styles.dim.Render("status:  "))
	b.WriteString(m.styles.status[t.Status].Render(string(t.Status)))
	b.WriteString("\n")
	field("updated", t.UpdatedAt.Format("2006-01-02 15:04:05"))
	if t.PRURL != "" {
		b.WriteString(m.styles.dim.Render("pr:  "))
		b.WriteString(m.styles.pr.Render(t.PRURL))
		b.WriteString("\n")
	}

	if t.TaskPrompt != "" {
		b.WriteString("\n")
		b.WriteString(m.styles.dim.Render("task:"))
		b.WriteString("\n")
		b.WriteString(wrapText(t.TaskPrompt, width-4))
		b.WriteString("\n")
	}

	if t.LastOutput != "" {
		b.WriteString("\n")
		title := "pane snapshot"
		if t.AwaitingInput {
			title = "Claude is asking — enter to attach"
		}
		// Hand-roll the frame so we own the width math instead of
		// relying on lipgloss's border + Width interaction (which
		// silently breaks on lines containing East-Asian-Width or
		// box-drawing characters).
		b.WriteString(m.renderFrame(title, t.LastOutput, width-4, t.AwaitingInput))
	}

	return lipgloss.NewStyle().Width(width).Padding(1, 2).Render(b.String())
}

// renderFrame draws a rounded box around content. width is the total
// outer width including border characters. The title is inlined into
// the top border. Content lines are soft-wrapped to fit; trailing
// padding is computed in display columns (runewidth) so unicode box
// characters and emoji don't break the right border.
func (m *model) renderFrame(title, content string, width int, hot bool) string {
	if width < 10 {
		width = 10
	}
	innerWidth := width - 4 // "│ " + " │"

	borderStyle := m.styles.snippetBorder
	if !hot {
		borderStyle = m.styles.dim
	}

	var b strings.Builder

	// Top border with inline title.
	titleSegment := "╭─ " + title + " "
	pad := width - runewidth.StringWidth(titleSegment) - 1
	if pad < 0 {
		pad = 0
	}
	b.WriteString(borderStyle.Render(titleSegment + strings.Repeat("─", pad) + "╮"))
	b.WriteString("\n")

	for _, raw := range strings.Split(content, "\n") {
		for _, line := range wrapToWidth(raw, innerWidth) {
			padCols := innerWidth - runewidth.StringWidth(line)
			if padCols < 0 {
				padCols = 0
			}
			b.WriteString(borderStyle.Render("│ "))
			b.WriteString(m.styles.snippetText.Render(line))
			b.WriteString(strings.Repeat(" ", padCols))
			b.WriteString(borderStyle.Render(" │"))
			b.WriteString("\n")
		}
	}

	// Bottom border.
	b.WriteString(borderStyle.Render("╰" + strings.Repeat("─", width-2) + "╯"))
	return b.String()
}

// wrapToWidth breaks line at width display columns, preferring word
// boundaries. Returns at least one slice element (the original line)
// when the input is already short enough.
func wrapToWidth(line string, width int) []string {
	if width <= 0 || runewidth.StringWidth(line) <= width {
		return []string{line}
	}
	words := strings.Fields(line)
	if len(words) == 0 {
		return []string{line}
	}
	var out []string
	current := ""
	for _, w := range words {
		if current == "" {
			current = w
			continue
		}
		if runewidth.StringWidth(current)+1+runewidth.StringWidth(w) > width {
			out = append(out, current)
			current = w
			continue
		}
		current += " " + w
	}
	if current != "" {
		out = append(out, current)
	}
	return out
}

// wrapText soft-wraps s at width, preserving paragraph breaks.
// Just enough wrapping for the right panel's task field.
func wrapText(s string, width int) string {
	if width <= 0 {
		return s
	}
	var out strings.Builder
	for _, para := range strings.Split(s, "\n") {
		if para == "" {
			out.WriteString("\n")
			continue
		}
		line := ""
		for _, word := range strings.Fields(para) {
			if line == "" {
				line = word
				continue
			}
			if len(line)+1+len(word) > width {
				out.WriteString(line)
				out.WriteString("\n")
				line = word
				continue
			}
			line += " " + word
		}
		if line != "" {
			out.WriteString(line)
			out.WriteString("\n")
		}
	}
	return strings.TrimRight(out.String(), "\n")
}

func shortID(id string) string {
	if len(id) <= 15 {
		return id
	}
	return id[len(id)-15:]
}

func joinRepos(rs []state.TrackRepo) string {
	names := make([]string, 0, len(rs))
	for _, r := range rs {
		names = append(names, r.Name)
	}
	return strings.Join(names, ",")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}

func padRight(s string, n int) string {
	if len(s) >= n {
		return s[:n]
	}
	return s + strings.Repeat(" ", n-len(s))
}
