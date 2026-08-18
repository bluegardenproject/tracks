package dashboard

import (
	"fmt"
	"sort"
	"strings"

	"github.com/bluegardenproject/tracks/internal/daemon"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// The dashboard Proxy tab is a read-only status view: it lists the defined
// stable ports and, for each, the running dev server it currently forwards
// to. Interaction — adding/removing ports and linking them to servers —
// lives in the dedicated Proxy screen (the menu's "Proxy" action), so there
// is a single place that owns proxy edits.

// updateProxy handles key input while the Proxy tab is active. Read-only:
// navigation, refresh, and the shared tab/quit keys only.
func (m *model) updateProxy(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	n := len(m.proxies)
	if m.proxyCursor >= n {
		m.proxyCursor = max(0, n-1)
	}
	if m.proxyCursor < 0 {
		m.proxyCursor = 0
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
		if m.proxyCursor < n-1 {
			m.proxyCursor++
		}
	}
	return m, nil
}

// viewProxy renders the Proxy tab: a selectable list of the defined stable
// ports and the dev server each forwards to.
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

	// --- footer (fixed) ---
	footerLines := []string{
		"",
		m.styles.dim.Render("manage ports from the menu (Proxy) — add, remove, link"),
		m.styles.dim.Render("↑/↓ select   tab tracks   r refresh   q quit"),
	}

	rows := sortedProxies(m.proxies)
	if m.proxyCursor >= len(rows) {
		m.proxyCursor = max(0, len(rows)-1)
	}
	if m.proxyCursor < 0 {
		m.proxyCursor = 0
	}

	used := len(lines) + len(footerLines)
	bodyBudget := budget - used
	if bodyBudget < 1 {
		bodyBudget = 1
	}

	if m.err != nil {
		lines = append(lines, m.styles.dim.Render("daemon unreachable: ")+m.err.Error())
	} else if len(rows) == 0 {
		lines = append(lines, m.styles.dim.Render("no stable ports defined — add one from the menu (Proxy)"))
	} else {
		lines = append(lines, m.styles.header.Render(fmt.Sprintf("  %-8s  %-28s  %-6s", "PORT", "UPSTREAM", "BIND")))
		lines = append(lines, m.renderProxyPortsWindow(rows, bodyBudget-1)...)
	}
	if m.statusMsg != "" {
		lines = append(lines, "", m.styles.warn.Render(m.statusMsg))
	}

	lines = append(lines, footerLines...)

	out := strings.Join(lines, "\n")
	out = clampLines(out, budget)
	if width > 0 {
		out = lipgloss.NewStyle().MaxWidth(width).Render(out)
	}
	return out
}

// renderProxyPortsWindow renders the port rows as a scrolling window (with
// "↑/↓ N more" indicators) that keeps proxyCursor visible within budget
// lines. Mirrors the tracks table's windowing via visibleRowWindow.
func (m *model) renderProxyPortsWindow(rows []daemon.ProxyEntryStatus, budget int) []string {
	n := len(rows)
	if budget < 1 {
		budget = 1
	}
	if n <= budget {
		out := make([]string, 0, n)
		for i, r := range rows {
			out = append(out, m.renderProxyPort(i, r))
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
		out = append(out, m.renderProxyPort(i, rows[i]))
	}
	if end < n {
		out = append(out, m.styles.dim.Render(fmt.Sprintf("  ↓ %d more", n-end)))
	}
	return out
}

// renderProxyPort renders one port row; the cursor row is highlighted.
func (m *model) renderProxyPort(i int, p daemon.ProxyEntryStatus) string {
	var upstream string
	if p.Upstream == "" {
		upstream = m.styles.dim.Render("(free)")
	} else if p.ActiveTrackID != "" {
		target := shortID(p.ActiveTrackID)
		if p.ActiveService != "" {
			target += "/" + p.ActiveService
		}
		upstream = m.styles.ok.Render("→ " + target)
	} else {
		upstream = m.styles.ok.Render("→ " + p.Upstream)
	}
	bind := ""
	if p.BindAll {
		bind = m.styles.warn.Render("all")
	}
	line := "  " +
		padRight(fmt.Sprintf(":%d", p.PublicPort), 8) + "  " +
		padRendered(upstream, 28) + "  " +
		padRendered(bind, 6)
	if i == m.proxyCursor {
		return m.styles.rowActive.Render(line)
	}
	return line
}

// sortedProxies returns the proxy entries ordered by public port so the
// list is stable frame to frame.
func sortedProxies(ps []daemon.ProxyEntryStatus) []daemon.ProxyEntryStatus {
	out := make([]daemon.ProxyEntryStatus, len(ps))
	copy(out, ps)
	sort.Slice(out, func(i, j int) bool { return out[i].PublicPort < out[j].PublicPort })
	return out
}
