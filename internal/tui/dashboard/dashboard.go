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
)

// pollInterval is the cadence at which the dashboard re-reads
// daemon state. One second is fast enough to feel live but slow
// enough to keep CPU near-idle.
const pollInterval = 1 * time.Second

// styles holds all lipgloss styles. Centralized so a future theme
// switch is a single edit.
type styles struct {
	header    lipgloss.Style
	row       lipgloss.Style
	rowActive lipgloss.Style
	status    map[state.Status]lipgloss.Style
	prompt    lipgloss.Style
	dim       lipgloss.Style
	pr        lipgloss.Style
}

func defaultStyles() styles {
	return styles{
		header:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")),
		row:       lipgloss.NewStyle(),
		rowActive: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Background(lipgloss.Color("236")),
		status: map[state.Status]lipgloss.Style{
			state.StatusPending: lipgloss.NewStyle().Foreground(lipgloss.Color("11")),
			state.StatusRunning: lipgloss.NewStyle().Foreground(lipgloss.Color("10")),
			state.StatusWaiting: lipgloss.NewStyle().Foreground(lipgloss.Color("11")),
			state.StatusDone:    lipgloss.NewStyle().Foreground(lipgloss.Color("8")),
			state.StatusErrored: lipgloss.NewStyle().Foreground(lipgloss.Color("9")),
		},
		prompt: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Background(lipgloss.Color("3")).Padding(0, 1),
		dim:    lipgloss.NewStyle().Foreground(lipgloss.Color("8")),
		pr:     lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Underline(true),
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

func (m *model) View() string {
	var b strings.Builder

	// Header.
	b.WriteString(m.styles.header.Render("tracks — dashboard"))
	b.WriteString("  ")
	b.WriteString(m.styles.dim.Render(fmt.Sprintf("(%d tracks, %d pending prompts)", len(m.tracks), len(m.prompts))))
	b.WriteString("\n\n")

	// Pending prompts banner.
	for _, p := range m.prompts {
		b.WriteString(m.styles.prompt.Render(fmt.Sprintf("APPROVAL  %s wants %s — %s   [y=allow / n=deny]",
			shortID(p.TrackID), p.Tool, p.Detail)))
		b.WriteString("\n")
	}
	if len(m.prompts) > 0 {
		b.WriteString("\n")
	}

	// Tracks table.
	if m.err != nil {
		b.WriteString(m.styles.dim.Render("daemon unreachable: ") + m.err.Error() + "\n")
	} else if len(m.tracks) == 0 {
		b.WriteString(m.styles.dim.Render("no tracks yet — run `tracks new`\n"))
	} else {
		b.WriteString(m.styles.header.Render(fmt.Sprintf("  %-15s  %-30s  %-10s  %-30s  %s",
			"ID", "BRANCH", "STATUS", "REPOS", "PR")))
		b.WriteString("\n")
		for i, t := range m.tracks {
			line := fmt.Sprintf("  %-15s  %-30s  %s  %-30s  %s",
				shortID(t.ID),
				truncate(t.Branch, 30),
				m.styles.status[t.Status].Render(padRight(string(t.Status), 10)),
				truncate(joinRepos(t.Repos), 30),
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

	// Footer.
	b.WriteString("\n")
	b.WriteString(m.styles.dim.Render("↑/↓ select  enter switch window  y/n answer prompt  r refresh  q quit"))
	return b.String()
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
