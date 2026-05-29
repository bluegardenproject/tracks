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

	// info is non-nil while the per-track info modal is open. The
	// modal hides the rest of the table view. Set by `i`, cleared
	// by `esc`.
	info *info
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
	// tmux uses the hostname as the default pane title for any
	// pane whose program doesn't emit an OSC title sequence.
	// Claude's TUI sets its own title in track panes; our Go
	// process doesn't, so without this override the dashboard
	// pane's border would just show the user's hostname.
	_ = m.tmux.SetCurrentPaneTitle("Dashboard")
	return tea.Batch(m.poll(), tickEvery())
}

// windowNameFor returns the tmux window name a track's claude
// pane lives in. Kept in sync with the daemon-side helper of the
// same name.
func windowNameFor(trackID string) string {
	if len(trackID) >= 6 {
		return "t-" + trackID[len(trackID)-6:]
	}
	return "t-" + trackID
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
		// The info modal swallows keystrokes so navigation in the
		// table doesn't happen while the popup is open.
		if m.info != nil {
			switch msg.String() {
			case "esc", "i", "q", "ctrl+c":
				m.info = nil
				return m, nil
			case "enter":
				t := m.info.track
				m.info = nil
				_ = m.attachTrack(t.ID)
				return m, nil
			case "r":
				m.info = openInfo(m.cfg, m.info.track)
				return m, nil
			}
			return m, nil
		}
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
		case "i":
			if len(m.tracks) > 0 {
				m.info = openInfo(m.cfg, m.tracks[m.cursor])
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
// track has been ended. Best-effort — failure is silent because
// the daemon side has already mutated state.
func (m *model) closeTrackWindow(trackID string) {
	window := windowNameFor(trackID)
	if exists, _ := m.tmux.HasWindow(m.cfg.Tmux.SessionName, window); !exists {
		return
	}
	_ = m.tmux.KillWindow(m.cfg.Tmux.SessionName, window)
}

// attachTrack switches focus to the track's tmux window.
func (m *model) attachTrack(trackID string) error {
	session := m.cfg.Tmux.SessionName
	window := windowNameFor(trackID)
	exists, _ := m.tmux.HasWindow(session, window)
	if !exists {
		return fmt.Errorf("window %s missing — claude likely exited", window)
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
	if m.info != nil {
		return m.renderInfo(m.info)
	}
	return m.renderTable(m.width)
}

// renderTable draws the dashboard's table + footer. The claude
// pane for the selected track lives in its own tmux pane below
// (joined in by syncEmbedded) — we don't render any details
// here.
func (m *model) renderTable(width int) string {
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
		b.WriteString(m.styles.header.Render(fmt.Sprintf("  %-15s  %-22s  %-16s  %-10s  %-14s  %-6s  %-16s  %s",
			"ID", "BRANCH", "SLUG", "STATUS", "CHANGES", "IDLE", "REPOS", "PR")))
		b.WriteString("\n")
		for i, t := range m.tracks {
			line := fmt.Sprintf("  %-15s  %-22s  %-16s  %s  %-14s  %-6s  %-16s  %s",
				shortID(t.ID),
				truncate(t.Branch, 22),
				truncate(t.Slug, 16),
				m.styles.status[t.Status].Render(padRight(string(t.Status), 10)),
				renderChanges(t.Changes),
				renderIdle(t),
				truncate(joinRepos(t.Repos), 16),
				m.renderPRCell(t),
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
	b.WriteString(m.styles.dim.Render("↑/↓ select   enter attach   i info   d end   K kill   x forget   X clear completed   y/n approve"))
	b.WriteString("\n")
	b.WriteString(m.styles.dim.Render("r refresh   q quit   (open menu from any window with <prefix>+t)"))
	return lipgloss.NewStyle().Width(width).Render(b.String())
}

func shortID(id string) string {
	if len(id) <= 15 {
		return id
	}
	return id[len(id)-15:]
}

// renderChanges turns a state.Changes into the dashboard's compact
// `+ins -del (N)` form. Empty when there's nothing to show.
func renderChanges(c state.Changes) string {
	if c.IsZero() {
		return ""
	}
	return fmt.Sprintf("+%d -%d (%d)", c.Insertions, c.Deletions, c.Files)
}

// renderPRCell builds the right-hand PR column: URL + a state
// badge + an optional comment count. Color picks a hint that
// matches the badge (pink for changes-requested, green for
// approved, etc.).
func (m *model) renderPRCell(t state.Track) string {
	if t.PRURL == "" {
		return ""
	}
	url := m.styles.pr.Render(t.PRURL)
	badge := prBadge(t)
	if badge != "" {
		badge = m.prBadgeStyle(t).Render(" [" + badge + "]")
	}
	count := ""
	if t.PRComments > 0 {
		count = m.styles.dim.Render(fmt.Sprintf(" (%d comments)", t.PRComments))
	}
	return url + badge + count
}

// prBadge returns the short label shown after the URL.
func prBadge(t state.Track) string {
	if t.PRDraft {
		return "draft"
	}
	switch t.PRState {
	case "MERGED":
		return "merged"
	case "CLOSED":
		return "closed"
	}
	switch t.PRReviewState {
	case "APPROVED":
		return "approved"
	case "CHANGES_REQUESTED":
		return "changes-requested"
	case "REVIEW_REQUIRED":
		return "review-required"
	}
	if t.PRState == "OPEN" {
		return "open"
	}
	return ""
}

// prBadgeStyle picks the lipgloss style for the badge based on
// the track's PR state. Falls back to dim when we don't have a
// specific opinion.
func (m *model) prBadgeStyle(t state.Track) lipgloss.Style {
	switch {
	case t.PRDraft:
		return m.styles.dim
	case t.PRState == "MERGED":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("13")) // purple
	case t.PRState == "CLOSED":
		return m.styles.dim
	case t.PRReviewState == "APPROVED":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("10")) // green
	case t.PRReviewState == "CHANGES_REQUESTED":
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("207")) // pink
	case t.PRReviewState == "REVIEW_REQUIRED":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("11")) // yellow
	default:
		return m.styles.dim
	}
}

// renderIdle returns a short "time since last update" string for a
// track. Terminal tracks just show their final state's age. Running
// tracks see a live count.
func renderIdle(t state.Track) string {
	if t.UpdatedAt.IsZero() {
		return ""
	}
	d := time.Since(t.UpdatedAt)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
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
