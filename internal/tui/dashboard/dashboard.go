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
	"os/exec"
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

	// embeddedTrackID is the ID of the track whose claude pane is
	// currently joined into the dashboard window. Empty when no
	// pane is embedded.
	embeddedTrackID string
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
	// On startup, restore any pane that was left embedded from a
	// previous dashboard process (e.g. it crashed mid-embed).
	// PaneTitle on pane index 1 holds the track id we last
	// joined — if found, break it back to its own window so we
	// start clean.
	if title, _ := m.tmux.PaneTitle(m.cfg.Tmux.SessionName, dashboardWindow, 1); title != "" {
		_ = m.tmux.BreakPane(m.cfg.Tmux.SessionName, dashboardWindow, 1, "t-"+lastN(title, 6))
	}
	return tea.Batch(m.poll(), tickEvery())
}

// dashboardWindow is the tmux window name the dashboard runs in.
// Must match what bootstrap creates.
const dashboardWindow = "Dashboard"

// embedHeightPct is how much of the dashboard window the embedded
// claude pane consumes. 65% leaves the table comfortably visible
// while Claude still has room for its TUI.
const embedHeightPct = 65

// syncEmbedded ensures the currently selected track's claude pane
// is the one embedded in the dashboard window. Called whenever the
// cursor moves.
//
// Behavior:
//   - cursor on an active track with a live window → embed that pane
//   - cursor on a terminal track or no track → restore embed (back
//     to its own window), leaving the dashboard with one pane
//   - same track as before → no-op
func (m *model) syncEmbedded() {
	if len(m.tracks) == 0 {
		m.restoreEmbedded()
		return
	}
	sel := m.tracks[m.cursor]
	if sel.Status.IsTerminal() {
		m.restoreEmbedded()
		return
	}
	if sel.ID == m.embeddedTrackID {
		return
	}
	srcWindow := windowNameFor(sel.ID)
	if exists, _ := m.tmux.HasWindow(m.cfg.Tmux.SessionName, srcWindow); !exists {
		// No live window for this track (Claude already exited).
		m.restoreEmbedded()
		return
	}
	// Restore any previous embed first.
	m.restoreEmbedded()
	if err := m.tmux.JoinPane(m.cfg.Tmux.SessionName, srcWindow, dashboardWindow, embedHeightPct); err != nil {
		return
	}
	_ = m.tmux.SetPaneTitle(m.cfg.Tmux.SessionName, dashboardWindow, 1, sel.ID)
	m.embeddedTrackID = sel.ID
}

// restoreEmbedded breaks the embedded pane (if any) back to its
// own t-<id> window so the dashboard window has one pane again.
func (m *model) restoreEmbedded() {
	if m.embeddedTrackID == "" {
		return
	}
	id := m.embeddedTrackID
	m.embeddedTrackID = ""
	_ = m.tmux.BreakPane(m.cfg.Tmux.SessionName, dashboardWindow, 1, windowNameFor(id))
}

func windowNameFor(trackID string) string {
	return "t-" + lastN(trackID, 6)
}

func lastN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
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
			// Restore any embedded track to its own window before
			// exiting — otherwise the next dashboard launch would
			// inherit a stranded pane.
			m.restoreEmbedded()
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m.syncEmbedded()
			}
		case "down", "j":
			if m.cursor < len(m.tracks)-1 {
				m.cursor++
				m.syncEmbedded()
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
//
// If the track is currently embedded in the dashboard window, we
// have to release the embed first or the kill-window call would
// take the dashboard pane with it.
func (m *model) closeTrackWindow(trackID string) {
	if m.embeddedTrackID == trackID {
		m.restoreEmbedded()
	}
	window := windowNameFor(trackID)
	if exists, _ := m.tmux.HasWindow(m.cfg.Tmux.SessionName, window); !exists {
		return
	}
	_ = m.tmux.KillWindow(m.cfg.Tmux.SessionName, window)
}

// attachTrack switches focus to the embedded pane (or the track's
// own window if it isn't currently embedded). The dashboard auto-
// embeds whatever's selected, so in practice `enter` just moves
// the user into the embedded pane to start typing.
func (m *model) attachTrack(trackID string) error {
	session := m.cfg.Tmux.SessionName
	if m.embeddedTrackID == trackID {
		// Already embedded — focus the bottom pane.
		cmd := exec.Command("tmux", "select-pane", "-t", session+":"+dashboardWindow+".1")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("select-pane: %w: %s", err, string(out))
		}
		return nil
	}
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
		b.WriteString(m.styles.header.Render(fmt.Sprintf("  %-15s  %-22s  %-16s  %-10s  %-18s  %s",
			"ID", "BRANCH", "SLUG", "STATUS", "REPOS", "PR")))
		b.WriteString("\n")
		for i, t := range m.tracks {
			line := fmt.Sprintf("  %-15s  %-22s  %-16s  %s  %-18s  %s",
				shortID(t.ID),
				truncate(t.Branch, 22),
				truncate(t.Slug, 16),
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
