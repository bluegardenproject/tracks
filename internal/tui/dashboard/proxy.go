package dashboard

import (
	"fmt"
	"sort"
	"strings"

	"github.com/bluegardenproject/tracks/internal/daemon"
	"github.com/bluegardenproject/tracks/internal/state"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// proxyRow is one live dev-server instance (a track's running service),
// annotated with the stable proxy it can be linked to (if the service
// declares a proxy_port) and whether it's the current upstream.
type proxyRow struct {
	trackID    string
	trackLabel string
	service    string
	trackPort  int
	proxyPort  int  // 0 when the service has no configured proxy_port
	active     bool // this instance is the proxy's current upstream
}

// proxyRows flattens the running dev servers across all tracks into the
// selectable rows of the Proxy view, cross-referenced with proxy status.
func (m *model) proxyRows() []proxyRow {
	byService := make(map[string]daemon.ProxyEntryStatus, len(m.proxies))
	for _, p := range m.proxies {
		byService[p.ServiceName] = p
	}
	var rows []proxyRow
	for _, t := range m.tracks {
		for _, sv := range t.Services {
			if !sv.Status.Live() {
				continue
			}
			r := proxyRow{
				trackID:    t.ID,
				trackLabel: trackLabel(t),
				service:    sv.Name,
				trackPort:  sv.Port,
			}
			if pe, ok := byService[sv.Name]; ok {
				r.proxyPort = pe.PublicPort
				r.active = pe.ActiveTrackID == t.ID
			}
			rows = append(rows, r)
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].service != rows[j].service {
			return rows[i].service < rows[j].service
		}
		return rows[i].trackLabel < rows[j].trackLabel
	})
	return rows
}

// trackLabel is the short identifier shown for a track in the Proxy view:
// its slug when set, otherwise the short ID.
func trackLabel(t state.Track) string {
	if t.Slug != "" {
		return truncate(t.Slug, 8)
	}
	return shortID(t.ID)
}

// updateProxy handles key input while the Proxy tab is active.
func (m *model) updateProxy(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	rows := m.proxyRows()
	if m.proxyCursor >= len(rows) {
		m.proxyCursor = max(0, len(rows)-1)
	}
	if m.proxyCursor < 0 {
		m.proxyCursor = 0
	}
	sel := func() *proxyRow {
		if m.proxyCursor >= 0 && m.proxyCursor < len(rows) {
			return &rows[m.proxyCursor]
		}
		return nil
	}

	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "r":
		return m, m.poll()
	case "up", "k":
		if m.proxyCursor > 0 {
			m.proxyCursor--
		}
	case "down", "j":
		if m.proxyCursor < len(rows)-1 {
			m.proxyCursor++
		}
	case "enter", "l":
		// Link: point the service's stable port at this track's server.
		if r := sel(); r != nil {
			if r.proxyPort == 0 {
				m.statusMsg = r.service + " has no proxy_port configured"
			} else {
				_ = m.client.ProxySwitch(r.service, r.trackID)
				return m, m.poll()
			}
		}
	case "f":
		// Free: clear the proxy upstream and release the fixed port.
		if r := sel(); r != nil {
			if r.proxyPort == 0 {
				m.statusMsg = r.service + " has no proxy_port to free"
			} else {
				_ = m.client.ProxySwitch(r.service, "off")
				return m, m.poll()
			}
		}
	case "s":
		// Stop the selected dev server.
		if r := sel(); r != nil {
			m.statusMsg = "stopping " + r.service + "…"
			return m, m.stopServer(r.trackID, r.service)
		}
	}
	return m, nil
}

// viewProxy renders the Proxy tab: a summary of each configured proxy
// (fixed port -> active track) above a selectable table of the dev servers
// currently running across all tracks.
func (m *model) viewProxy() string {
	width := m.width
	budget := m.height
	unconstrained := budget <= 0
	if unconstrained {
		budget = 1 << 30
	}

	var lines []string
	lines = append(lines, strings.Split(bigBanner("PROXY"), "\n")...)
	lines = append(lines, "")

	// --- proxy summary (informational) ---
	if len(m.proxies) == 0 {
		lines = append(lines, m.styles.dim.Render("no proxy_port configured — add `proxy_port: <N>` to a service in ~/.config/tracks/config.yaml"))
	} else {
		lines = append(lines, m.styles.sectionHdr.Render("Proxies"))
		for _, p := range sortedProxies(m.proxies) {
			var target string
			if p.Upstream == "" {
				target = m.styles.dim.Render("(free)")
			} else if p.ActiveTrackID != "" {
				target = m.styles.ok.Render("→ " + shortID(p.ActiveTrackID))
			} else {
				target = m.styles.ok.Render("→ " + p.Upstream)
			}
			lines = append(lines, "  "+padRight(p.ServiceName, 14)+padRight(fmt.Sprintf(":%d", p.PublicPort), 8)+target)
		}
	}
	if m.statusMsg != "" {
		lines = append(lines, "", m.styles.warn.Render(m.statusMsg))
	}
	lines = append(lines, "")

	// --- footer (fixed) ---
	footerLines := []string{
		"",
		m.styles.dim.Render("↑/↓ select   enter/l link to proxy   f free port   s stop server"),
		m.styles.dim.Render("tab tracks   r refresh   q quit"),
	}

	// --- running-servers table gets the remaining height ---
	rows := m.proxyRows()
	used := len(lines) + len(footerLines)
	bodyBudget := budget - used
	if bodyBudget < 1 {
		bodyBudget = 1
	}

	if m.err != nil {
		lines = append(lines, m.styles.dim.Render("daemon unreachable: ")+m.err.Error())
	} else if len(rows) == 0 {
		lines = append(lines, m.styles.dim.Render("no dev servers running — start some from the Tracks tab (u) or `tracks up`"))
	} else {
		if m.proxyCursor >= len(rows) {
			m.proxyCursor = len(rows) - 1
		}
		if m.proxyCursor < 0 {
			m.proxyCursor = 0
		}
		lines = append(lines, m.styles.header.Render(fmt.Sprintf("  %-10s  %-16s  %-11s  %-8s  %-6s",
			"TRACK", "SERVICE", "TRACK PORT", "PROXY", "ACTIVE")))
		// The column header consumed one line of the body budget; the rest
		// is a scrolling window that always keeps proxyCursor visible and
		// leaves the fixed footer on screen.
		lines = append(lines, m.renderProxyRowsWindow(rows, bodyBudget-1)...)
	}

	lines = append(lines, footerLines...)

	out := strings.Join(lines, "\n")
	out = clampLines(out, budget)
	if width > 0 {
		out = lipgloss.NewStyle().MaxWidth(width).Render(out)
	}
	return out
}

// renderProxyRowsWindow renders the running-server rows as a scrolling
// window (with "↑/↓ N more" indicators) that keeps proxyCursor visible
// within budget lines. Mirrors renderRows for the tracks table, reusing
// visibleRowWindow.
func (m *model) renderProxyRowsWindow(rows []proxyRow, budget int) []string {
	n := len(rows)
	if budget < 1 {
		budget = 1
	}
	if n <= budget {
		out := make([]string, 0, n)
		for i, r := range rows {
			out = append(out, m.renderProxyRow(i, r))
		}
		return out
	}
	capacity := budget - 2
	if capacity < 1 {
		capacity = 1
	}
	start, end := visibleRowWindow(n, m.proxyCursor, capacity)
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
	start, end = visibleRowWindow(n, m.proxyCursor, capacity)

	var out []string
	if start > 0 {
		out = append(out, m.styles.dim.Render(fmt.Sprintf("  ↑ %d more", start)))
	}
	for i := start; i < end; i++ {
		out = append(out, m.renderProxyRow(i, rows[i]))
	}
	if end < n {
		out = append(out, m.styles.dim.Render(fmt.Sprintf("  ↓ %d more", n-end)))
	}
	return out
}

// renderProxyRow renders one running-server row; the cursor row is
// highlighted. Styled cells are width-padded with padRendered so the ANSI
// escape codes don't throw off column alignment.
func (m *model) renderProxyRow(i int, r proxyRow) string {
	proxyCell := m.styles.dim.Render("—")
	if r.proxyPort != 0 {
		proxyCell = fmt.Sprintf(":%d", r.proxyPort)
	}
	activeCell := ""
	if r.active {
		activeCell = m.styles.ok.Render("● live")
	}
	line := "  " +
		padRight(r.trackLabel, 10) + "  " +
		padRight(truncate(r.service, 16), 16) + "  " +
		padRight(fmt.Sprintf("%d", r.trackPort), 11) + "  " +
		padRendered(proxyCell, 8) + "  " +
		padRendered(activeCell, 6)
	if i == m.proxyCursor {
		return m.styles.rowActive.Render(line)
	}
	return line
}

// sortedProxies returns the proxy entries ordered by service name so the
// summary list is stable frame to frame.
func sortedProxies(ps []daemon.ProxyEntryStatus) []daemon.ProxyEntryStatus {
	out := make([]daemon.ProxyEntryStatus, len(ps))
	copy(out, ps)
	sort.Slice(out, func(i, j int) bool { return out[i].ServiceName < out[j].ServiceName })
	return out
}
