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
	"github.com/bluegardenproject/tracks/internal/usage"
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
	header     lipgloss.Style
	panelTitle lipgloss.Style
	sectionHdr lipgloss.Style
	row        lipgloss.Style
	rowActive  lipgloss.Style
	status     map[state.Status]lipgloss.Style
	prompt     lipgloss.Style
	dim        lipgloss.Style
	pr         lipgloss.Style
	branch     lipgloss.Style
	slug       lipgloss.Style
	repo       lipgloss.Style
	insertions lipgloss.Style
	deletions  lipgloss.Style
	count      lipgloss.Style
	cost       lipgloss.Style
	ok         lipgloss.Style
	warn       lipgloss.Style
	fail       lipgloss.Style
	panel      lipgloss.Style
}

func defaultStyles() styles {
	return styles{
		header:     lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")),
		panelTitle: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("207")),
		sectionHdr: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14")),
		row:        lipgloss.NewStyle(),
		rowActive:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Background(lipgloss.Color("236")),
		status: map[state.Status]lipgloss.Style{
			state.StatusPending: lipgloss.NewStyle().Foreground(lipgloss.Color("11")),
			state.StatusRunning: lipgloss.NewStyle().Foreground(lipgloss.Color("10")),
			// Hot pink — a waiting track is blocking the developer
			// and should jump out of the table.
			state.StatusWaiting: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("207")),
			// Blue — Claude is done and the PR is open for review. Not
			// blocking the dev like Waiting, but still an active track.
			state.StatusPR: lipgloss.NewStyle().Foreground(lipgloss.Color("12")),
			// Amber — a finished track, readable on both light and dark
			// terminals and clearly distinct from the other statuses.
			state.StatusDone:    lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "130", Dark: "215"}),
			state.StatusErrored: lipgloss.NewStyle().Foreground(lipgloss.Color("9")),
			// Gray — a saved draft, not yet launched. Deliberately muted so
			// it reads as inert next to the active and end-state colors.
			state.StatusDraft: lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "244", Dark: "244"}),
		},
		prompt: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Background(lipgloss.Color("3")).Padding(0, 1),
		// AdaptiveColor picks at render time: a mid-dark gray on
		// light terminals (where ANSI 8 turns nearly invisible)
		// and a lighter gray on dark terminals. Same code path
		// for both, no theme-specific configuration.
		dim:        lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "240", Dark: "245"}),
		pr:         lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Underline(true),
		branch:     lipgloss.NewStyle().Foreground(lipgloss.Color("10")),
		slug:       lipgloss.NewStyle().Foreground(lipgloss.Color("13")),
		repo:       lipgloss.NewStyle().Foreground(lipgloss.Color("14")),
		insertions: lipgloss.NewStyle().Foreground(lipgloss.Color("10")),
		deletions:  lipgloss.NewStyle().Foreground(lipgloss.Color("9")),
		count:      lipgloss.NewStyle().Foreground(lipgloss.Color("11")),
		cost:       lipgloss.NewStyle().Foreground(lipgloss.Color("78")),
		ok:         lipgloss.NewStyle().Foreground(lipgloss.Color("10")),
		warn:       lipgloss.NewStyle().Foreground(lipgloss.Color("11")),
		fail:       lipgloss.NewStyle().Foreground(lipgloss.Color("9")),
		panel: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("14")).
			Padding(0, 1),
	}
}

// Run launches the dashboard in the current terminal. Blocks until
// the user quits.
func Run(cfg config.Config) error {
	m := newModel(cfg)
	_, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

// viewMode selects which tab the dashboard is showing.
type viewMode int

const (
	modeTracks viewMode = iota // the track list (default)
	modeProxy                  // the stable-port proxy + running-servers view
)

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

	// mode is the active tab; toggled with Tab.
	mode viewMode
	// proxies is the last-polled stable-port proxy status, shown in the
	// Proxy tab. proxyCursor is the selected running-server row there.
	proxies     []daemon.ProxyEntryStatus
	proxyCursor int

	// detail is the lazily-refreshed extra info shown below the
	// table for whatever row the cursor's on. Refreshed every
	// poll; cleared when there are no tracks.
	detail *detail

	// statusMsg is a transient one-line message for operation
	// feedback (e.g., a failed resume). Unlike m.err it does not
	// hide the track table. Cleared on the next successful poll.
	statusMsg string
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
	proxies []daemon.ProxyEntryStatus
	err     error
}

// resumeResult is what the resume goroutine returns.
type resumeResult struct {
	windowName string
	err        error
}

// launchResult is what the launch goroutine returns.
type launchResult struct {
	windowName string
	err        error
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

// refreshDetail re-runs gatherDetail for the currently-highlighted
// track, or clears m.detail when there's nothing selected. Called
// after every cursor move and on each daemon-state poll.
func (m *model) refreshDetail() {
	if len(m.tracks) == 0 || m.cursor >= len(m.tracks) {
		m.detail = nil
		return
	}
	d := gatherDetail(m.cfg, m.tracks[m.cursor])
	m.detail = &d
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
		// Prompts and proxy status are best-effort; a failure in either
		// must not blank the whole dashboard.
		prompts, _ := cl.PendingPrompts()
		ps, _ := cl.ProxyStatus()
		return pollResult{tracks: tracks, prompts: prompts, proxies: ps.Proxies}
	}
}

func (m *model) resumeTrack(id string) tea.Cmd {
	cfg := m.cfg
	return func() tea.Msg {
		cl := daemon.NewClient(cfg)
		res, err := cl.Resume(id)
		if err != nil {
			return resumeResult{err: err}
		}
		return resumeResult{windowName: res.WindowName}
	}
}

func (m *model) launchTrack(id string) tea.Cmd {
	cfg := m.cfg
	return func() tea.Msg {
		cl := daemon.NewClient(cfg)
		res, err := cl.LaunchWithProgress(id, nil)
		if err != nil {
			return launchResult{err: err}
		}
		return launchResult{windowName: res.WindowName}
	}
}

// serversResult reports the outcome of a start-all / stop dev-server action.
type serversResult struct {
	verb string // "start" or "stop", for the status message
	err  error
}

// startServers boots every configured dev server for a track (the daemon
// opens one pane per service). Runs async because deps install + boot can
// take a moment; progress shows in the panes, not here.
func (m *model) startServers(id string) tea.Cmd {
	cfg := m.cfg
	return func() tea.Msg {
		cl := daemon.NewClient(cfg)
		_, err := cl.ServiceUpWithProgress(id, "", nil)
		return serversResult{verb: "start", err: err}
	}
}

// stopServer stops one running dev server (runs its pre_stop hooks first).
func (m *model) stopServer(trackID, service string) tea.Cmd {
	cfg := m.cfg
	return func() tea.Msg {
		cl := daemon.NewClient(cfg)
		err := cl.ServiceDownWithProgress(trackID, service, nil)
		return serversResult{verb: "stop", err: err}
	}
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		// Tab toggles between the Tracks and Proxy views regardless of mode.
		if msg.String() == "tab" {
			if m.mode == modeTracks {
				m.mode = modeProxy
			} else {
				m.mode = modeTracks
			}
			return m, nil
		}
		if m.mode == modeProxy {
			return m.updateProxy(msg)
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m.refreshDetail()
			}
		case "down", "j":
			if m.cursor < len(m.tracks)-1 {
				m.cursor++
				m.refreshDetail()
			}
		case "enter":
			if len(m.tracks) > 0 {
				t := m.tracks[m.cursor]
				_ = m.attachTrack(t)
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
			// Forget the highlighted track. Valid for a terminal track or a
			// saved draft (neither has a live process); a draft dismissed
			// here is the same as choosing "Dismiss" at creation time.
			if len(m.tracks) > 0 {
				t := m.tracks[m.cursor]
				if t.Status.IsTerminal() || t.Status == state.StatusDraft {
					_ = m.client.Forget(t.ID)
					return m, m.poll()
				}
			}
		case "X":
			// Prune every completed track.
			_, _ = m.client.PruneCompleted()
			return m, m.poll()
		case "d":
			// Graceful end of the highlighted track. Valid whether
			// it's still active or already finished: a finished track
			// keeps its pane alive as a shell (see ShellCommand), and
			// "d" is how the user tears that down. The daemon removes
			// the worktree and closes the tmux window as part of Done.
			// A draft has nothing to end — it's launched (L) or
			// dismissed (x), so End/Kill skip it (the daemon refuses it
			// too, but guarding here gives a clear hint).
			if len(m.tracks) > 0 {
				t := m.tracks[m.cursor]
				if t.Status == state.StatusDraft {
					m.statusMsg = "draft — press L to launch or x to dismiss"
				} else {
					_ = m.client.Done(t.ID)
					return m, m.poll()
				}
			}
		case "K":
			// Force kill the highlighted track. Like "d" but SIGKILL.
			// Capital K to distinguish from lowercase k (cursor-up
			// vim convention) and to make accidental kills harder.
			// Also valid on a finished track to close its lingering
			// shell window. A draft is launched (L) / dismissed (x).
			if len(m.tracks) > 0 {
				t := m.tracks[m.cursor]
				if t.Status == state.StatusDraft {
					m.statusMsg = "draft — press L to launch or x to dismiss"
				} else {
					_ = m.client.Kill(t.ID)
					return m, m.poll()
				}
			}
		case "u":
			// Start all of the highlighted track's dev servers, daemon-side
			// (no Claude involved). The daemon opens one pane per service.
			if len(m.tracks) > 0 {
				t := m.tracks[m.cursor]
				if t.Status.IsTerminal() {
					m.statusMsg = "track is finished — can't start its servers"
				} else {
					m.statusMsg = "starting servers for " + shortID(t.ID) + "…"
					return m, m.startServers(t.ID)
				}
			}
		case "r":
			return m, m.poll()
		case "R":
			// Resume the highlighted track (must be terminal with a session ID).
			if len(m.tracks) > 0 {
				t := m.tracks[m.cursor]
				if t.Status.IsTerminal() && t.SessionID != "" {
					return m, m.resumeTrack(t.ID)
				}
			}
		case "L":
			// Launch the highlighted draft (or failed-creation) track from
			// its saved parameters — (re)runs creation.
			if len(m.tracks) > 0 {
				t := m.tracks[m.cursor]
				if t.CanLaunch() {
					m.statusMsg = "launching draft…"
					return m, m.launchTrack(t.ID)
				}
			}
		}
	case tickMsg:
		return m, tea.Batch(m.poll(), tickEvery())
	case pollResult:
		m.lastPoll = time.Now()
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.err = nil
			m.statusMsg = ""
			// Re-anchor the cursor on the same track by ID before we
			// swap in the new slice. Selection is otherwise positional,
			// so a list that grew or shrank (new / forgotten / pruned
			// track) would silently slide the highlight onto a different
			// track without the user moving it.
			var selID string
			if m.cursor >= 0 && m.cursor < len(m.tracks) {
				selID = m.tracks[m.cursor].ID
			}
			m.tracks = msg.tracks
			m.prompts = msg.prompts
			m.proxies = msg.proxies
			if selID != "" {
				for i, t := range m.tracks {
					if t.ID == selID {
						m.cursor = i
						break
					}
				}
			}
			if m.cursor >= len(m.tracks) {
				m.cursor = max(0, len(m.tracks)-1)
			}
			if m.cursor < 0 {
				m.cursor = 0
			}
			m.refreshDetail()
		}
	case resumeResult:
		if msg.err != nil {
			m.statusMsg = "resume failed: " + msg.err.Error()
		} else if msg.windowName != "" {
			_ = m.tmux.SelectWindow(m.cfg.Tmux.SessionName, msg.windowName)
		}
		return m, m.poll()
	case launchResult:
		if msg.err != nil {
			m.statusMsg = "launch failed: " + msg.err.Error()
		} else if msg.windowName != "" {
			_ = m.tmux.SelectWindow(m.cfg.Tmux.SessionName, msg.windowName)
		}
		return m, m.poll()
	case serversResult:
		if msg.err != nil {
			m.statusMsg = msg.verb + " servers failed: " + msg.err.Error()
		} else {
			m.statusMsg = ""
		}
		return m, m.poll()
	}
	return m, nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// attachTrack switches focus to the track's tmux window.
func (m *model) attachTrack(t state.Track) error {
	session := m.cfg.Tmux.SessionName
	window := t.WindowName()
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

// View assembles the whole dashboard as a frame that is never taller
// than the terminal and never wider than it. Bubble Tea's renderer
// garbles any frame that overflows the window — the table's highlight
// detaches from the visible rows and the selection appears "lost" —
// so every section is budgeted against m.height and the result is
// hard-clamped in both dimensions as a final safety net.
func (m *model) View() string {
	if m.mode == modeProxy {
		return m.viewProxy()
	}
	width := m.width
	budget := m.height
	// Before the first WindowSizeMsg (or in a degenerate size) we don't
	// yet know the window; render everything unclamped for that one
	// frame rather than guess. A resize message follows almost
	// immediately under WithAltScreen.
	unconstrained := budget <= 0
	if unconstrained {
		budget = 1 << 30
	}

	// --- fixed chrome above the table ---
	var lines []string
	lines = append(lines, strings.Split(bigBanner("TRACKS"), "\n")...)
	lines = append(lines, "")
	lines = append(lines, m.styles.dim.Render(fmt.Sprintf("%d tracks · %d pending prompts", len(m.tracks), len(m.prompts))))
	for _, p := range m.prompts {
		lines = append(lines, m.styles.prompt.Render(fmt.Sprintf("APPROVAL  %s wants %s — %s   [y=allow / n=deny]",
			shortID(p.TrackID), p.Tool, p.Detail)))
	}
	if m.statusMsg != "" {
		lines = append(lines, m.styles.warn.Render(m.statusMsg))
	}
	lines = append(lines, "")

	// --- footer (fixed) ---
	footerLines := []string{
		"",
		m.styles.dim.Render("↑/↓ select   enter attach   u start servers   d end   K kill   x forget   X clear completed   y/n approve"),
		m.styles.dim.Render("tab proxy   r refresh   R resume   L launch draft   q quit   (open menu from any window with <prefix>+t)"),
	}

	// --- detail panel (below footer), height-capped ---
	var detailLines []string
	if m.detail != nil && len(m.tracks) > 0 {
		maxDetail := -1 // unclamped
		show := unconstrained
		if !unconstrained {
			// Give the detail panel at most ~40% of the window so it can
			// never starve the table of rows; drop it entirely when the
			// window is too short to show a useful panel.
			maxDetail = budget * 2 / 5
			show = maxDetail >= 8
		}
		if show {
			ds := m.renderDetail(*m.detail, width, maxDetail)
			detailLines = append([]string{""}, strings.Split(ds, "\n")...)
		}
	}

	// --- table body gets whatever vertical space is left ---
	used := len(lines) + len(footerLines) + len(detailLines)
	rowsBudget := budget - used
	if rowsBudget < 1 {
		rowsBudget = 1
	}
	if m.err != nil {
		lines = append(lines, m.styles.dim.Render("daemon unreachable: ")+m.err.Error())
	} else if len(m.tracks) == 0 {
		lines = append(lines, m.styles.dim.Render("no tracks yet — run `tracks new`"))
	} else {
		lines = append(lines, m.styles.header.Render(fmt.Sprintf("  %-15s  %-7s  %-28s  %-26s  %-10s  %-22s  %-5s  %-8s",
			"ID", "KIND", "BRANCH", "SLUG", "STATUS", "CHANGES", "SVC", "COST")))
		// The header consumes one row of the budget; the rest is the
		// scrolling window of track rows.
		if rows := m.renderRows(rowsBudget - 1); rows != "" {
			lines = append(lines, strings.Split(rows, "\n")...)
		}
	}

	lines = append(lines, footerLines...)
	lines = append(lines, detailLines...)

	out := strings.Join(lines, "\n")
	// Hard bounds: never taller or wider than the window. clampLines
	// guards any budgeting miscount; MaxWidth stops an over-wide row
	// from soft-wrapping into extra terminal lines (which would defeat
	// the height budget).
	out = clampLines(out, budget)
	if width > 0 {
		out = lipgloss.NewStyle().MaxWidth(width).Render(out)
	}
	return out
}

// renderRows renders the track table body as a scrolling window that
// always keeps the selected row visible, fitting within budget lines
// (including any "↑ N more" / "↓ N more" indicator lines). Callers
// pass the space left after the fixed chrome.
func (m *model) renderRows(budget int) string {
	n := len(m.tracks)
	if n == 0 {
		return ""
	}
	if budget < 1 {
		budget = 1
	}

	// Everything fits: no scrolling, no indicators.
	if n <= budget {
		rows := make([]string, 0, n)
		for i, t := range m.tracks {
			rows = append(rows, m.renderRow(i, t))
		}
		return strings.Join(rows, "\n")
	}

	// Scrolling needed. Reserve up to two lines for indicators, then
	// reclaim one if only a single indicator ends up showing (cursor at
	// an edge). Two passes suffice: extra capacity can only remove
	// indicators, never add them.
	capacity := budget - 2
	if capacity < 1 {
		capacity = 1
	}
	start, end := visibleRowWindow(n, m.cursor, capacity)
	reserved := 0
	if start > 0 {
		reserved++
	}
	if end < n {
		reserved++
	}
	capacity = budget - reserved
	if capacity < 1 {
		capacity = 1
	}
	start, end = visibleRowWindow(n, m.cursor, capacity)

	var rows []string
	if start > 0 {
		rows = append(rows, m.styles.dim.Render(fmt.Sprintf("  ↑ %d more", start)))
	}
	for i := start; i < end; i++ {
		rows = append(rows, m.renderRow(i, m.tracks[i]))
	}
	if end < n {
		rows = append(rows, m.styles.dim.Render(fmt.Sprintf("  ↓ %d more", n-end)))
	}
	return strings.Join(rows, "\n")
}

// visibleRowWindow returns the [start,end) range of row indices to show
// so that cursor stays visible within at most capacity rows. capacity
// must be >= 1. The window is centered on the cursor and clamped to the
// list ends.
func visibleRowWindow(n, cursor, capacity int) (start, end int) {
	if capacity >= n {
		return 0, n
	}
	start = cursor - capacity/2
	if start < 0 {
		start = 0
	}
	end = start + capacity
	if end > n {
		end = n
		start = end - capacity
	}
	if start < 0 {
		start = 0
	}
	return start, end
}

// renderRow renders a single track row. The row at the cursor gets the
// highlight background threaded through every cell (see the inline note
// below); all others render plainly.
func (m *model) renderRow(i int, t state.Track) string {
	branch := t.Branch
	if branch == "" {
		branch = "—"
	}
	if i != m.cursor {
		return fmt.Sprintf("  %-15s  %s  %s  %s  %s  %s  %s  %s",
			shortID(t.ID),
			padRendered(m.renderKind(t), 7),
			padRendered(m.styles.branch.Render(truncate(branch, 28)), 28),
			padRendered(m.styles.slug.Render(truncate(t.Slug, 26)), 26),
			m.styles.status[t.Status].Render(padRight(string(t.Status), 10)),
			padRendered(m.renderChangesColored(t.Changes), 22),
			padRendered(m.renderServices(t), 5),
			padRendered(m.renderCost(t.Usage), 8),
		)
	}

	// Thread the highlight background into every cell individually.
	// Without this, each cell's own Render() appends a hard ANSI
	// reset that cancels the outer rowActive background before the
	// next cell starts, leaving all but the first column unlit.
	activeBg := lipgloss.Color("236")
	addBg := func(s lipgloss.Style) lipgloss.Style {
		return s.Background(activeBg)
	}
	pad := func(s string, width int) string {
		return lipgloss.NewStyle().Width(width).Background(activeBg).Render(s)
	}
	sep := lipgloss.NewStyle().Background(activeBg).Render("  ")

	// KIND: inline to apply bg to the text, not just the outer container.
	k := t.Kind
	if k == "" {
		k = state.KindWork
	}
	var kColor lipgloss.TerminalColor
	switch k {
	case state.KindAsk, state.KindPlan:
		kColor = lipgloss.Color("13")
	case state.KindReview:
		kColor = lipgloss.Color("11")
	default:
		kColor = lipgloss.AdaptiveColor{Light: "30", Dark: "51"}
	}
	kindStr := lipgloss.NewStyle().Foreground(kColor).Background(activeBg).Render(string(k))

	// CHANGES: each sub-segment needs bg so inter-segment spaces stay lit.
	var changesStr string
	if !t.Changes.IsZero() {
		changesStr = addBg(m.styles.insertions).Render(fmt.Sprintf("+%d", t.Changes.Insertions)) +
			addBg(lipgloss.NewStyle()).Render(" ") +
			addBg(m.styles.deletions).Render(fmt.Sprintf("-%d", t.Changes.Deletions)) +
			addBg(lipgloss.NewStyle()).Render(" ") +
			addBg(m.styles.dim).Render(fmt.Sprintf("(%d)", t.Changes.Files))
	}

	// COST: apply bg to the inner style so the value text is highlighted.
	var costStr string
	if t.Usage.IsZero() {
		costStr = addBg(m.styles.dim).Render("—")
	} else {
		costStr = addBg(m.styles.cost).Render(usage.FormatCost(t.Usage.CostUSD))
	}

	// SVC: apply bg to preserve selection highlight.
	var svcStr string
	if live, total := svcCounts(t); total > 0 {
		sv := fmt.Sprintf("%d/%d", live, total)
		if live > 0 {
			svcStr = addBg(m.styles.insertions).Render(sv)
		} else {
			svcStr = addBg(m.styles.dim).Render(sv)
		}
	}

	return m.styles.rowActive.Render(fmt.Sprintf("  %-15s", shortID(t.ID))) +
		sep + pad(kindStr, 7) +
		sep + pad(addBg(m.styles.branch).Render(truncate(branch, 28)), 28) +
		sep + pad(addBg(m.styles.slug).Render(truncate(t.Slug, 26)), 26) +
		sep + addBg(m.styles.status[t.Status]).Render(padRight(string(t.Status), 10)) +
		sep + pad(changesStr, 22) +
		sep + pad(svcStr, 5) +
		sep + pad(costStr, 8)
}

// clampLines truncates s to at most max newline-separated lines. A
// negative max leaves s untouched.
func clampLines(s string, max int) string {
	if max < 0 {
		return s
	}
	lines := strings.Split(s, "\n")
	if len(lines) <= max {
		return s
	}
	return strings.Join(lines[:max], "\n")
}

func shortID(id string) string {
	if len(id) <= 15 {
		return id
	}
	return id[len(id)-15:]
}

// renderChanges turns a state.Changes into the dashboard's compact
// `+ins -del (N)` form (plain text). Empty when there's nothing
// to show. Kept around for non-coloured contexts (the daemon doesn't
// know our styles).
func renderChanges(c state.Changes) string {
	if c.IsZero() {
		return ""
	}
	return fmt.Sprintf("+%d -%d (%d)", c.Insertions, c.Deletions, c.Files)
}

// renderKind renders a track's kind as a short colored badge.
// Read-only kinds (ask/plan) get a distinct color so they're easy to
// pick out from editable work/review tracks. Empty kind (pre-migration)
// reads as work.
func (m *model) renderKind(t state.Track) string {
	k := t.Kind
	if k == "" {
		k = state.KindWork
	}
	var color lipgloss.TerminalColor
	switch k {
	case state.KindAsk, state.KindPlan:
		color = lipgloss.Color("13") // magenta — read-only
	case state.KindReview:
		color = lipgloss.Color("11") // yellow
	default:
		// Teal — readable on both themes and distinct from the magenta
		// (ask/plan) and yellow (review) kinds.
		color = lipgloss.AdaptiveColor{Light: "30", Dark: "51"} // work
	}
	return lipgloss.NewStyle().Foreground(color).Render(string(k))
}

// renderChangesColored is the dashboard-styled variant: green
// insertions, red deletions, yellow file count. Empty when zero.
func (m *model) renderChangesColored(c state.Changes) string {
	if c.IsZero() {
		return ""
	}
	return m.styles.insertions.Render(fmt.Sprintf("+%d", c.Insertions)) +
		" " + m.styles.deletions.Render(fmt.Sprintf("-%d", c.Deletions)) +
		" " + m.styles.dim.Render(fmt.Sprintf("(%d)", c.Files))
}

// renderCost renders a track's USD cost for the COST column. Dim
// placeholder until the first assistant turn produces usage.
func (m *model) renderCost(u state.Usage) string {
	if u.IsZero() {
		return m.styles.dim.Render("—")
	}
	return m.styles.cost.Render(usage.FormatCost(u.CostUSD))
}

// padRendered pads a (possibly already-styled) string to width
// display columns. lipgloss.NewStyle().Width handles ANSI escape
// sequences correctly, where Sprintf("%-Ns", ...) would count
// escape bytes and underpad.
func padRendered(s string, width int) string {
	return lipgloss.NewStyle().Width(width).Render(s)
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

// svcCounts returns (live, total) service counts for a track.
// total prefers len(t.Ports) (authoritative once port allocation runs)
// but falls back to len(t.Services) for tracks created before the
// port-allocation feature shipped, so the SVC column is never blank
// when services are actually running.
// Width assumption: realistic values are at most two digits each ("99/99" = 5
// chars), matching the SVC column width of 5.
func svcCounts(t state.Track) (live, total int) {
	total = max(len(t.Ports), len(t.Services))
	for _, s := range t.Services {
		if s.Status.Live() {
			live++
		}
	}
	return
}

// renderServices returns a styled "live/total" port count for the SVC column.
// Returns "" when no ports are configured for the track.
// Live services are highlighted green; none running is dim.
func (m *model) renderServices(t state.Track) string {
	live, total := svcCounts(t)
	if total == 0 {
		return ""
	}
	s := fmt.Sprintf("%d/%d", live, total)
	if live > 0 {
		return m.styles.insertions.Render(s)
	}
	return m.styles.dim.Render(s)
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
