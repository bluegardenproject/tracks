package tracksview

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/ui/source"
	"github.com/bluegardenproject/tracks/internal/ui/widget"
)

// proxyEvery is how often the proxy's ports are read again: fresh while
// the Proxy tab shows, else the daemon's last view, for the tab's mark.
const proxyEvery = 2 * time.Second

// proxyTick starts the next read's clock; tests stop it.
var proxyTick = tea.Tick

// proxyTab is the Proxy tab's state.
type proxyTab struct {
	view   source.ProxyView
	err    error
	loaded bool
	// selected is a port's row, or len(ports) for the add button.
	selected int
	// adding shows a new port's row, its input focused.
	adding   bool
	input    textinput.Model
	inputErr string
	offset   int // lines scrolled away
	hover    proxyHit
	notice   notice
	// picking is the port the open picker sets the input of.
	picking int
	// loading says a read is under way, so ticks don't stack reads;
	// changes counts the changes that landed, so a read that started
	// before one is dropped.
	loading bool
	changes int
}

// live reports whether a port forwards to a running server: the tab's
// mark.
func (p proxyTab) live() bool {
	for _, st := range p.view.Ports {
		if st.State == source.ProxyForwarding {
			return true
		}
	}
	return false
}

type (
	proxyMsg struct {
		view  source.ProxyView
		err   error
		fresh bool
		seq   int // changes when the read started
	}
	proxyTickMsg    struct{}
	proxyChangedMsg struct {
		view  source.ProxyView
		err   error
		added int // the port added, 0 for another change
	}
)

func newProxyTab() proxyTab {
	in := widget.NewInput(5, "3000")
	return proxyTab{input: in, hover: proxyHit{kind: hitNone}}
}

// loadProxy reads the ports: fresh with what runs now and the servers
// to pick from, else the daemon's last view.
func (m Model) loadProxy(fresh bool) (Model, tea.Cmd) {
	if m.proxySource == nil {
		return m, nil
	}
	m.proxy.loading = true
	src, seq := m.proxySource, m.proxy.changes
	return m, func() tea.Msg {
		v, err := src.View(context.Background(), fresh)
		return proxyMsg{v, err, fresh, seq}
	}
}

// nextProxyTick schedules the next read.
func (m Model) nextProxyTick() tea.Cmd {
	if m.proxySource == nil {
		return nil
	}
	return proxyTick(proxyEvery, func(time.Time) tea.Msg { return proxyTickMsg{} })
}

// proxyTicked reads the ports again, unless the last read is still
// under way.
func (m Model) proxyTicked() (Model, tea.Cmd) {
	if m.proxy.loading {
		return m, m.nextProxyTick()
	}
	m, load := m.loadProxy(m.tab == tabProxy)
	return m, tea.Batch(load, m.nextProxyTick())
}

func (m Model) setProxy(msg proxyMsg) Model {
	p := &m.proxy
	p.loading = false
	if msg.seq != p.changes {
		return m // a change landed since the read started
	}
	if msg.err != nil {
		if msg.fresh {
			p.err = msg.err
		}
		return m
	}
	inputs := p.view.Inputs
	p.view, p.err, p.loaded = msg.view, nil, true
	if !msg.fresh {
		p.view.Inputs = inputs
	}
	p.selected = min(p.selected, len(p.view.Ports))
	m = m.refreshProxyPicker()
	return m.clampProxy()
}

// proxyChanged shows what a change left, or why it failed. A change
// saved before the proxy caught up counts as made; the next read shows
// it.
func (m Model) proxyChanged(msg proxyChangedMsg) (Model, tea.Cmd) {
	p := &m.proxy
	p.changes++
	saved := msg.err != nil && strings.HasPrefix(msg.err.Error(), "saved, but")
	if msg.err != nil && !saved {
		if msg.added != 0 {
			p.inputErr = msg.err.Error()
		} else {
			p.notice = notice{text: msg.err.Error(), err: true}
		}
		return m, nil
	}
	if saved {
		p.notice = notice{text: msg.err.Error(), err: true}
		if msg.added != 0 {
			m = m.stopAdding()
		}
		return m.loadProxy(m.tab == tabProxy)
	}
	p.view, p.err, p.loaded = msg.view, nil, true
	if msg.added != 0 {
		p.adding, p.inputErr = false, ""
		p.input.Blur()
		p.input.SetValue("")
		for i, st := range p.view.Ports {
			if st.Port == msg.added {
				p.selected = i
			}
		}
		p.notice = notice{text: fmt.Sprintf("Added port %d. Pick what it forwards to.", msg.added)}
	}
	p.selected = min(p.selected, len(p.view.Ports))
	return m.scrollProxy(), nil
}

// change runs fn on the proxy source and reports its result.
func (m Model) change(added int, fn func(ctx context.Context, src source.Proxy) (source.ProxyView, error)) tea.Cmd {
	if m.proxySource == nil {
		return nil
	}
	src := m.proxySource
	return func() tea.Msg {
		v, err := fn(context.Background(), src)
		return proxyChangedMsg{v, err, added}
	}
}

// startAdding shows a new port's row with its input focused.
func (m Model) startAdding() (Model, tea.Cmd) {
	p := &m.proxy
	p.adding, p.inputErr = true, ""
	p.input.SetValue("")
	p.selected = len(p.view.Ports)
	m = m.scrollProxy()
	return m, p.input.Focus()
}

func (m Model) stopAdding() Model {
	p := &m.proxy
	p.adding, p.inputErr = false, ""
	p.input.Blur()
	p.input.SetValue("")
	return m.scrollProxy()
}

// submitPort adds the port typed in.
func (m Model) submitPort() (Model, tea.Cmd) {
	p := &m.proxy
	port, err := strconv.Atoi(strings.TrimSpace(p.input.Value()))
	if err != nil {
		p.inputErr = "Enter a port number, such as 3000."
		return m, nil
	}
	return m, m.change(port, func(ctx context.Context, src source.Proxy) (source.ProxyView, error) { return src.Add(ctx, port) })
}

// removePort removes the port on row i.
func (m Model) removePort(i int) (Model, tea.Cmd) {
	if i < 0 || i >= len(m.proxy.view.Ports) {
		return m, nil
	}
	port := m.proxy.view.Ports[i].Port
	m.proxy.notice = notice{text: fmt.Sprintf("Removed port %d.", port)}
	return m, m.change(0, func(ctx context.Context, src source.Proxy) (source.ProxyView, error) { return src.Remove(ctx, port) })
}

// proxyKey handles a key on the Proxy tab. ok is false for keys it
// doesn't use.
func (m Model) proxyKey(msg tea.KeyPressMsg) (_ Model, _ tea.Cmd, ok bool) {
	p := &m.proxy
	key := msg.String()
	if p.adding {
		switch key {
		case "esc":
			return m.stopAdding(), nil, true
		case "tab", "shift+tab":
			return m.stopAdding(), nil, false
		case "enter":
			next, cmd := m.submitPort()
			return next, cmd, true
		}
		var cmd tea.Cmd
		p.input, cmd = p.input.Update(msg)
		p.input.SetValue(digits(p.input.Value()))
		p.inputErr = ""
		return m, cmd, true
	}
	switch key {
	case "up", "k":
		p.selected = max(0, p.selected-1)
	case "down", "j":
		p.selected = min(len(p.view.Ports), p.selected+1)
	case "enter", "space":
		if p.selected == len(p.view.Ports) {
			next, cmd := m.startAdding()
			return next, cmd, true
		}
		next, cmd := m.openProxyPicker(p.selected)
		return next, cmd, true
	case "n":
		next, cmd := m.startAdding()
		return next, cmd, true
	case "delete", "backspace":
		next, cmd := m.removePort(p.selected)
		return next, cmd, true
	default:
		return m, nil, false
	}
	return m.scrollProxy(), nil, true
}

// proxyPaste gives the new port's input a paste.
func (m Model) proxyPaste(msg tea.Msg) (Model, tea.Cmd) {
	p := &m.proxy
	if !p.adding {
		return m, nil
	}
	var cmd tea.Cmd
	p.input, cmd = p.input.Update(msg)
	p.input.SetValue(digits(p.input.Value()))
	return m, cmd
}

// digits keeps s's digits.
func digits(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, s)
}

// openProxyPicker lists what row i's port can forward to.
func (m Model) openProxyPicker(i int) (Model, tea.Cmd) {
	if i < 0 || i >= len(m.proxy.view.Ports) {
		return m, nil
	}
	m.proxy.selected, m.proxy.picking = i, m.proxy.view.Ports[i].Port
	m.pickerFor = pickProxy
	items, marked := m.proxyPickerItems()
	p := widget.NewPicker(fmt.Sprintf("Port %d forwards to", m.proxy.picking), items, marked)
	m.picker = &p
	return m.loadProxy(true)
}

// proxyPickerItems are none, then every running server, and the one
// the picked port forwards to.
func (m Model) proxyPickerItems() ([]widget.PickerItem, int) {
	items := []widget.PickerItem{{Label: "none", Detail: "forward to nothing"}}
	marked := -1
	var current source.ProxyInput
	for _, st := range m.proxy.view.Ports {
		if st.Port == m.proxy.picking {
			current = st.Input
		}
	}
	if current.None() {
		marked = 0
	}
	for i, sv := range m.proxy.view.Inputs {
		items = append(items, widget.PickerItem{Label: inputName(sv), Detail: m.serverDetail(sv)})
		if source.InputOf(sv) == current {
			marked = i + 1
		}
	}
	return items, marked
}

// refreshProxyPicker shows the servers read since the picker opened.
func (m Model) refreshProxyPicker() Model {
	if m.picker == nil || m.pickerFor != pickProxy {
		return m
	}
	items, marked := m.proxyPickerItems()
	m.picker.SetItems(items)
	m.picker.Marked = marked
	return m
}

// proxyPicked sets the selected port's input to what the picker chose.
func (m Model) proxyPicked(r widget.PickerResult) (Model, tea.Cmd) {
	switch r {
	case widget.PickerClosed:
		m.picker = nil
	case widget.PickerChosen:
		i := m.picker.Cursor
		m.picker = nil
		var in source.ProxyInput
		if i > 0 && i-1 < len(m.proxy.view.Inputs) {
			in = source.InputOf(m.proxy.view.Inputs[i-1])
		}
		port := m.proxy.picking
		return m, m.change(0, func(ctx context.Context, src source.Proxy) (source.ProxyView, error) {
			return src.SetInput(ctx, port, in)
		})
	}
	return m, nil
}

// inputName is what a server is called: its name, or its port for one
// Tracks didn't start.
func inputName(sv source.Server) string {
	if sv.Name != "" {
		return sv.Name
	}
	return fmt.Sprintf(":%d", sv.Port)
}

// serverDetail is a server's type, track and port.
func (m Model) serverDetail(sv source.Server) string {
	var parts []string
	if sv.Type != "" {
		parts = append(parts, sv.Type)
	}
	parts = append(parts, m.trackName(sv.Track, sv.TrackName))
	if sv.Port != 0 && sv.Name != "" {
		parts = append(parts, fmt.Sprintf(":%d", sv.Port))
	}
	return strings.Join(parts, " · ")
}

// inputLabel is what a port's input shows: its server as it runs, or
// what's stored once it doesn't.
func (m Model) inputLabel(st source.ProxyStatus) string {
	switch {
	case st.Server != nil:
		return inputName(*st.Server) + " · " + m.serverDetail(*st.Server)
	case st.Input.None():
		return "none"
	case st.Input.Server != "":
		return st.Input.Server + " · " + m.trackName(st.Input.Track, "")
	}
	return fmt.Sprintf(":%d · %s", st.Input.Port, m.trackName(st.Input.Track, ""))
}

// trackName is track id's name as Station shows it, else fallback, else
// the ID.
func (m Model) trackName(id, fallback string) string {
	for _, t := range m.station.tracks {
		if t.ID == id {
			return t.Shown()
		}
	}
	if fallback != "" {
		return fallback
	}
	return id
}
