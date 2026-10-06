package tracksview

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/theme"
	"github.com/bluegardenproject/tracks/internal/ui/source"
	"github.com/bluegardenproject/tracks/internal/ui/widget"
)

// addPortButton adds an output port; it stays under the last one.
var addPortButton = widget.NewButton("Add new proxy port", widget.ButtonDefault)

// What a click on the Proxy tab hits.
const (
	hitNone   = iota
	hitRow    // a port's row
	hitPicker // a port's input
	hitRemove // a port's ×
	hitAdd    // the add button
	hitInput  // the new port's input
)

// proxyHit is something clickable on the Proxy tab, in body lines and
// cells.
type proxyHit struct {
	kind, row  int // row is the port's index
	line, x, w int
}

// The parts of a port's row, in cells.
const (
	portCellW  = 10 // "[ 3000  ]"
	removeW    = 2  // " ×"
	pickerMinW = 16
)

// proxyFrame is where the Ports frame is: its left cell and width.
func (m Model) proxyFrame() (x, w int) {
	return stationLeft, max(0, m.width-2*stationLeft)
}

func (m Model) proxyView(width, height int) []string {
	switch {
	case m.proxySource == nil:
		return m.message(width, height, m.fg(theme.TextMuted).Render("No daemon."))
	case !m.proxy.loaded && m.proxy.err != nil:
		return m.message(width, height, m.fg(theme.StateDangerText).Render("Couldn't read the proxy: "+m.proxy.err.Error()))
	}
	_, fw := m.proxyFrame()
	body, _ := m.proxyBody(fw - 4)
	body = body[min(m.proxy.offset, len(body)):]
	title := "Ports"
	if m.proxy.err != nil {
		// What shows is the last view that could be read.
		title += " · couldn't read the proxy: " + m.proxy.err.Error()
	}
	framed := m.frame(title, theme.BorderDefault, body, fw, height)
	margin := strings.Repeat(" ", stationLeft)
	lines := make([]string, height)
	for i := range lines {
		lines[i] = pad(margin+framed[i], width)
	}
	return lines
}

// proxyBody draws the inside of the Ports frame, width cells wide: a
// framed row per port, the new port's row while adding, then the add
// button. It reports what's clickable.
func (m Model) proxyBody(width int) ([]string, []proxyHit) {
	p := m.proxy
	var lines []string
	var hits []proxyHit
	for i, st := range p.view.Ports {
		focused := !p.adding && p.selected == i
		inner, h := m.portRow(st, i, width-4)
		start := len(lines)
		hovered := p.hover.row == i && (p.hover.kind == hitRow || p.hover.kind == hitPicker || p.hover.kind == hitRemove)
		lines = append(lines, m.box(inner, width, focused || hovered)...)
		for line := start; line < start+3; line++ {
			hits = append(hits, proxyHit{kind: hitRow, row: i, line: line, x: 0, w: width})
		}
		for _, hit := range h {
			hit.line, hit.x = start+1, hit.x+2
			hits = append(hits, hit)
		}
	}
	if p.adding {
		inner := m.newPortRow(width - 4)
		start := len(lines)
		lines = append(lines, m.box(inner, width, true)...)
		hits = append(hits, proxyHit{kind: hitInput, line: start + 1, x: 2, w: portCellW})
		if p.inputErr != "" {
			lines = append(lines, m.wrap(theme.StateDangerText, p.inputErr, width)...)
		}
	}
	if len(lines) > 0 {
		lines = append(lines, "")
	}
	button := addPortButton
	button.Hover = (!p.adding && p.selected == len(p.view.Ports)) || p.hover.kind == hitAdd
	hits = append(hits, proxyHit{kind: hitAdd, line: len(lines), x: 0, w: button.Width()})
	lines = append(lines, button.View(m.palette))
	return lines, hits
}

// box frames inner, one line, in a full-width rounded border.
func (m Model) box(inner string, width int, focused bool) []string {
	border := m.fg(theme.BorderDefault)
	if focused {
		border = m.fg(theme.BorderFocus)
	}
	w := max(0, width-2)
	return []string{
		border.Render("╭" + strings.Repeat("─", w) + "╮"),
		border.Render("│") + " " + pad(inner, max(0, width-4)) + " " + border.Render("│"),
		border.Render("╰" + strings.Repeat("─", w) + "╯"),
	}
}

// portRow draws a port's row, width cells: the port, its state, its
// input and ×. Its hits are relative to the row's first cell.
func (m Model) portRow(st source.ProxyStatus, i, width int) (string, []proxyHit) {
	pickerW := min(max(pickerMinW, width*45/100), max(0, width-portCellW-removeW))
	stateW := max(0, width-portCellW-pickerW-removeW)
	port := m.fg(theme.BorderDefault).Render("[") + m.fg(theme.TextDefault).Bold(true).Render(pad(" "+strconv.Itoa(st.Port), portCellW-2)) +
		m.fg(theme.BorderDefault).Render("]")
	text, color := m.portState(st)
	state := lipgloss.PlaceHorizontal(stateW, lipgloss.Center, m.fg(color).Render(cut(text, max(0, stateW-2))))
	label := m.inputLabel(st)
	labelColor := theme.TextDefault
	if st.Input.None() {
		labelColor = theme.TextMuted
	}
	picker := m.fg(theme.BorderDefault).Render("[ ") + m.fg(labelColor).Render(pad(cut(label, max(0, pickerW-6)), max(0, pickerW-6))) +
		m.fg(theme.TextMuted).Render(" ▾") + m.fg(theme.BorderDefault).Render(" ]")
	remove := " " + m.fg(theme.StateDangerText).Render("×")
	hits := []proxyHit{
		{kind: hitPicker, row: i, x: portCellW + stateW, w: pickerW},
		{kind: hitRemove, row: i, x: portCellW + stateW + pickerW, w: removeW},
	}
	return port + state + picker + remove, hits
}

// newPortRow draws the new port's row: its input and how to finish.
func (m Model) newPortRow(width int) string {
	p := m.proxy
	bracket := theme.BorderFocus
	if p.inputErr != "" {
		bracket = theme.StateDangerText
	}
	in := widget.Input(m.palette, p.input, portCellW-2, bracket)
	hint := m.fg(theme.TextMuted).Render(cut("Enter adds it · Esc cancels", max(0, width-portCellW-2)))
	return in + "  " + hint
}

// portState is what a port does, in words, and its colour.
func (m Model) portState(st source.ProxyStatus) (string, theme.Token) {
	switch st.State {
	case source.ProxyForwarding:
		return "forwards to", theme.TextDefault
	case source.ProxyNoServer:
		return "no server ⚠", theme.StateWarningText
	case source.ProxyBlocked:
		if st.Holder != "" {
			return "blocked by " + st.Holder, theme.StateDangerText
		}
		return "blocked: its input is this port", theme.StateDangerText
	}
	return "forwards to", theme.TextMuted
}

// proxyHitAt is what's drawn at cell x, y.
func (m Model) proxyHitAt(x, y int) (proxyHit, bool) {
	if m.tab != tabProxy || m.picker != nil {
		return proxyHit{}, false
	}
	fx, fw := m.proxyFrame()
	top := m.contentTop() + 1 - m.proxy.offset
	_, hits := m.proxyBody(fw - 4)
	var found proxyHit
	ok := false
	for _, h := range hits {
		line := h.line - m.proxy.offset
		if line < 0 || line >= m.contentHeight()-2 {
			continue
		}
		if y == top+h.line && x >= fx+2+h.x && x < fx+2+h.x+h.w {
			// The narrowest hit wins: a picker over its row.
			if !ok || h.w < found.w {
				found, ok = h, true
			}
		}
	}
	return found, ok
}

func (m Model) proxyClick(x, y int) (Model, tea.Cmd) {
	h, ok := m.proxyHitAt(x, y)
	if !ok {
		return m, nil
	}
	m.proxy.notice = notice{}
	if m.proxy.adding && h.kind != hitInput {
		m = m.stopAdding()
	}
	switch h.kind {
	case hitRow:
		m.proxy.selected = h.row
	case hitPicker:
		return m.openProxyPicker(h.row)
	case hitRemove:
		m.proxy.selected = h.row
		return m.removePort(h.row)
	case hitAdd:
		return m.startAdding()
	}
	return m.scrollProxy(), nil
}

func (m Model) proxyHover(x, y int) Model {
	m.proxy.hover = proxyHit{kind: hitNone}
	if h, ok := m.proxyHitAt(x, y); ok {
		m.proxy.hover = h
	}
	return m
}

// scrollProxy scrolls the Ports frame so the selected row, or the new
// port's, shows.
func (m Model) scrollProxy() Model {
	visible := m.contentHeight() - 2
	if visible <= 0 {
		return m
	}
	_, fw := m.proxyFrame()
	body, hits := m.proxyBody(fw - 4)
	p := &m.proxy
	first, last := -1, -1
	for _, h := range hits {
		match := (p.adding && h.kind == hitInput) ||
			(!p.adding && p.selected < len(p.view.Ports) && h.kind == hitRow && h.row == p.selected) ||
			(!p.adding && p.selected == len(p.view.Ports) && h.kind == hitAdd)
		if !match {
			continue
		}
		if first < 0 || h.line < first {
			first = h.line
		}
		last = max(last, h.line)
	}
	if p.adding {
		first, last = max(0, first-1), len(body)-1
	}
	if first >= 0 {
		if first < p.offset {
			p.offset = first
		}
		if last >= p.offset+visible {
			p.offset = last - visible + 1
		}
	}
	p.offset = max(0, min(p.offset, len(body)-visible))
	return m
}

// clampProxy keeps the scroll inside the Ports frame's lines, without
// following the selection: a refresh mustn't undo the wheel.
func (m Model) clampProxy() Model {
	_, fw := m.proxyFrame()
	body, _ := m.proxyBody(fw - 4)
	m.proxy.offset = max(0, min(m.proxy.offset, len(body)-(m.contentHeight()-2)))
	return m
}

// scrollProxyBy scrolls the Ports frame by delta lines.
func (m Model) scrollProxyBy(delta int) Model {
	visible := m.contentHeight() - 2
	_, fw := m.proxyFrame()
	body, _ := m.proxyBody(fw - 4)
	m.proxy.offset = max(0, min(m.proxy.offset+delta, len(body)-visible))
	return m
}
