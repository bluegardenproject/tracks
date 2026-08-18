// Package proxymgr is the interactive stable-port proxy manager: a
// two-level bubbletea screen where the user defines output ports and points
// each at any running dev server across any track.
//
// Level 1 lists the defined ports and their current upstream; from here the
// user adds (a) or removes (d) a port, or drills into one (enter). Level 2
// lists the dev servers running across all tracks so the user can link the
// port to one (enter) or free it (f). All state lives in the daemon — this
// screen only drives it through the client.
package proxymgr

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/daemon"
	"github.com/bluegardenproject/tracks/internal/state"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Run launches the proxy manager in the current terminal. Blocks until the
// user quits.
func Run(cfg config.Config) error {
	m := newModel(cfg)
	m.reload()
	_, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

type screen int

const (
	screenPorts screen = iota // the defined-ports list
	screenPick                // pick a running server for the selected port
	screenAdd                 // enter a new port number
)

// srv is one running dev server across all tracks, a link candidate.
type srv struct {
	trackID    string
	trackLabel string
	service    string
	port       int
}

type model struct {
	cfg    config.Config
	screen screen

	proxies []daemon.ProxyEntryStatus
	servers []srv

	portCursor int
	pickCursor int
	selPort    int // the port being linked on screenPick

	input   textinput.Model
	bindAll bool

	status        string
	err           error
	width, height int

	styles styles
}

type styles struct {
	title    lipgloss.Style
	dim      lipgloss.Style
	ok       lipgloss.Style
	warn     lipgloss.Style
	rowSel   lipgloss.Style
	header   lipgloss.Style
	hintKey  lipgloss.Style
	hintText lipgloss.Style
}

func newModel(cfg config.Config) *model {
	ti := textinput.New()
	ti.Placeholder = "3000"
	ti.CharLimit = 5
	ti.Prompt = "port: "
	return &model{
		cfg:    cfg,
		screen: screenPorts,
		input:  ti,
		styles: styles{
			title:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14")),
			dim:      lipgloss.NewStyle().Foreground(lipgloss.Color("244")),
			ok:       lipgloss.NewStyle().Foreground(lipgloss.Color("10")),
			warn:     lipgloss.NewStyle().Foreground(lipgloss.Color("11")),
			rowSel:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(lipgloss.Color("14")),
			header:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("244")),
			hintKey:  lipgloss.NewStyle().Bold(true),
			hintText: lipgloss.NewStyle().Foreground(lipgloss.Color("244")),
		},
	}
}

func (m *model) Init() tea.Cmd { return nil }

// reload refreshes the defined ports and running-server list from the daemon.
func (m *model) reload() {
	cl := daemon.NewClient(m.cfg)
	ps, err := cl.ProxyStatus()
	if err != nil {
		m.err = err
		return
	}
	m.err = nil
	m.proxies = ps.Proxies
	sort.Slice(m.proxies, func(i, j int) bool { return m.proxies[i].PublicPort < m.proxies[j].PublicPort })

	tracks, err := cl.Ls()
	if err != nil {
		m.err = err
		return
	}
	m.servers = flattenServers(tracks)
}

// flattenServers collects every active dev server across tracks.
func flattenServers(tracks []state.Track) []srv {
	var out []srv
	for _, t := range tracks {
		for _, sv := range t.Services {
			if !sv.Active() {
				continue
			}
			out = append(out, srv{
				trackID:    t.ID,
				trackLabel: trackLabel(t),
				service:    sv.Name,
				port:       sv.Port,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].trackLabel != out[j].trackLabel {
			return out[i].trackLabel < out[j].trackLabel
		}
		return out[i].service < out[j].service
	})
	return out
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		switch m.screen {
		case screenAdd:
			return m.updateAdd(msg)
		case screenPick:
			return m.updatePick(msg)
		default:
			return m.updatePorts(msg)
		}
	}
	return m, nil
}

func (m *model) updatePorts(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	n := len(m.proxies)
	m.clampCursor(&m.portCursor, n)
	switch msg.String() {
	case "q", "esc", "ctrl+c":
		return m, tea.Quit
	case "r":
		m.reload()
	case "up", "k":
		if m.portCursor > 0 {
			m.portCursor--
		}
	case "down", "j":
		if m.portCursor < n-1 {
			m.portCursor++
		}
	case "a":
		m.input.SetValue("")
		m.input.Focus()
		m.bindAll = false
		m.status = ""
		m.screen = screenAdd
	case "d", "x":
		if p, ok := m.selectedPort(); ok {
			if err := daemon.NewClient(m.cfg).ProxyRemove(p.PublicPort); err != nil {
				m.status = err.Error()
			} else {
				m.status = fmt.Sprintf("removed :%d", p.PublicPort)
			}
			m.reload()
		}
	case "enter", "l":
		if p, ok := m.selectedPort(); ok {
			m.selPort = p.PublicPort
			m.pickCursor = m.currentUpstreamRow(p)
			m.status = ""
			m.screen = screenPick
		}
	}
	return m, nil
}

func (m *model) updateAdd(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		m.screen = screenPorts
		return m, nil
	case "tab":
		m.bindAll = !m.bindAll
		return m, nil
	case "enter":
		port, err := parsePort(m.input.Value())
		if err != nil {
			m.status = err.Error()
			return m, nil
		}
		if err := daemon.NewClient(m.cfg).ProxyAdd(port, m.bindAll); err != nil {
			m.status = err.Error()
			return m, nil
		}
		m.status = fmt.Sprintf("added :%d", port)
		m.screen = screenPorts
		m.reload()
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m *model) updatePick(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	n := len(m.servers)
	m.clampCursor(&m.pickCursor, n)
	switch msg.String() {
	case "esc", "q":
		m.screen = screenPorts
	case "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.pickCursor > 0 {
			m.pickCursor--
		}
	case "down", "j":
		if m.pickCursor < n-1 {
			m.pickCursor++
		}
	case "f", "d":
		// De-select: free the port.
		if err := daemon.NewClient(m.cfg).ProxySwitch(m.selPort, "off", ""); err != nil {
			m.status = err.Error()
		} else {
			m.status = fmt.Sprintf(":%d freed", m.selPort)
		}
		m.screen = screenPorts
		m.reload()
	case "enter", "l":
		if m.pickCursor >= 0 && m.pickCursor < n {
			s := m.servers[m.pickCursor]
			if err := daemon.NewClient(m.cfg).ProxySwitch(m.selPort, s.trackID, s.service); err != nil {
				m.status = err.Error()
			} else {
				m.status = fmt.Sprintf(":%d → %s/%s", m.selPort, s.trackLabel, s.service)
			}
			m.screen = screenPorts
			m.reload()
		}
	}
	return m, nil
}

func (m *model) selectedPort() (daemon.ProxyEntryStatus, bool) {
	if m.portCursor >= 0 && m.portCursor < len(m.proxies) {
		return m.proxies[m.portCursor], true
	}
	return daemon.ProxyEntryStatus{}, false
}

// currentUpstreamRow returns the servers-list index the port currently
// points at, or 0 when it is free or the target is not in the list.
func (m *model) currentUpstreamRow(p daemon.ProxyEntryStatus) int {
	for i, s := range m.servers {
		if s.trackID == p.ActiveTrackID && s.service == p.ActiveService {
			return i
		}
	}
	return 0
}

func (m *model) clampCursor(c *int, n int) {
	if *c >= n {
		*c = n - 1
	}
	if *c < 0 {
		*c = 0
	}
}

func (m *model) View() string {
	switch m.screen {
	case screenAdd:
		return m.viewAdd()
	case screenPick:
		return m.viewPick()
	default:
		return m.viewPorts()
	}
}

func (m *model) viewPorts() string {
	var b strings.Builder
	b.WriteString(m.styles.title.Render("Proxy — stable ports"))
	b.WriteString("\n\n")

	if m.err != nil {
		b.WriteString(m.styles.warn.Render("daemon: " + m.err.Error()))
		b.WriteString("\n\n")
	} else if len(m.proxies) == 0 {
		b.WriteString(m.styles.dim.Render("no stable ports defined — press a to add one"))
		b.WriteString("\n\n")
	} else {
		b.WriteString(m.styles.header.Render(fmt.Sprintf("  %-8s  %-30s  %-5s", "PORT", "UPSTREAM", "BIND")))
		b.WriteString("\n")
		for i, p := range m.proxies {
			b.WriteString(m.renderPortRow(i, p))
			b.WriteString("\n")
		}
	}
	b.WriteString(m.statusLine())
	b.WriteString(m.hints(
		hint{"↑/↓", "select"}, hint{"enter", "link"}, hint{"a", "add"},
		hint{"d", "remove"}, hint{"r", "refresh"}, hint{"q", "quit"},
	))
	return b.String()
}

func (m *model) renderPortRow(i int, p daemon.ProxyEntryStatus) string {
	upstream := "(free)"
	if p.Upstream != "" {
		target := p.Upstream
		if p.ActiveTrackID != "" {
			target = shortID(p.ActiveTrackID)
			if p.ActiveService != "" {
				target += "/" + p.ActiveService
			}
		}
		upstream = "→ " + target
	}
	bind := ""
	if p.BindAll {
		bind = "all"
	}
	line := fmt.Sprintf("  %-8s  %-30s  %-5s", fmt.Sprintf(":%d", p.PublicPort), truncate(upstream, 30), bind)
	if i == m.portCursor {
		return m.styles.rowSel.Render(line)
	}
	if p.Upstream != "" {
		return m.styles.ok.Render(line)
	}
	return m.styles.dim.Render(line)
}

func (m *model) viewPick() string {
	var b strings.Builder
	b.WriteString(m.styles.title.Render(fmt.Sprintf("Link :%d → running dev server", m.selPort)))
	b.WriteString("\n\n")
	if len(m.servers) == 0 {
		b.WriteString(m.styles.dim.Render("no dev servers running — start one with `tracks up`"))
		b.WriteString("\n\n")
	} else {
		b.WriteString(m.styles.header.Render(fmt.Sprintf("  %-12s  %-16s  %-10s", "TRACK", "SERVICE", "PORT")))
		b.WriteString("\n")
		for i, s := range m.servers {
			line := fmt.Sprintf("  %-12s  %-16s  %-10d", truncate(s.trackLabel, 12), truncate(s.service, 16), s.port)
			if i == m.pickCursor {
				b.WriteString(m.styles.rowSel.Render(line))
			} else {
				b.WriteString(line)
			}
			b.WriteString("\n")
		}
	}
	b.WriteString(m.statusLine())
	b.WriteString(m.hints(
		hint{"↑/↓", "select"}, hint{"enter", "link"}, hint{"f", "free port"}, hint{"esc", "back"},
	))
	return b.String()
}

func (m *model) viewAdd() string {
	var b strings.Builder
	b.WriteString(m.styles.title.Render("Add a stable port"))
	b.WriteString("\n\n")
	b.WriteString("  " + m.input.View())
	b.WriteString("\n\n")
	bindLabel := "loopback only"
	if m.bindAll {
		bindLabel = m.styles.warn.Render("all interfaces (network-exposed)")
	}
	b.WriteString("  bind: " + bindLabel + m.styles.dim.Render("   (tab to toggle)"))
	b.WriteString("\n")
	b.WriteString(m.statusLine())
	b.WriteString(m.hints(hint{"enter", "add"}, hint{"tab", "toggle bind"}, hint{"esc", "cancel"}))
	return b.String()
}

func (m *model) statusLine() string {
	if m.status == "" {
		return "\n"
	}
	return "\n" + m.styles.warn.Render("  "+m.status) + "\n"
}

type hint struct{ key, text string }

func (m *model) hints(hs ...hint) string {
	var parts []string
	for _, h := range hs {
		parts = append(parts, m.styles.hintKey.Render(h.key)+" "+m.styles.hintText.Render(h.text))
	}
	return "\n" + strings.Join(parts, m.styles.dim.Render("   ")) + "\n"
}

// parsePort parses a port, tolerating a leading colon.
func parsePort(s string) (int, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), ":")
	p, err := strconv.Atoi(s)
	if err != nil || p < 1 || p > 65535 {
		return 0, fmt.Errorf("invalid port %q: want 1–65535", s)
	}
	return p, nil
}

// trackLabel is the short identifier shown for a track: its slug when set,
// otherwise the short ID.
func trackLabel(t state.Track) string {
	if t.Slug != "" {
		return truncate(t.Slug, 12)
	}
	return shortID(t.ID)
}

// shortID trims a track ID to its random suffix.
func shortID(id string) string {
	if i := strings.LastIndex(id, "-"); i >= 0 && i+1 < len(id) {
		return id[i+1:]
	}
	return id
}

// truncate shortens s to at most n runes (rune-aware so a multibyte label
// never splits mid-rune and throws off column alignment), appending an
// ellipsis when it cuts.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}
