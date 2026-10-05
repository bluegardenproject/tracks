package tracksview

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/theme"
	"github.com/bluegardenproject/tracks/internal/ui/source"
)

func init() {
	proxyTick = func(time.Duration, func(time.Time) tea.Msg) tea.Cmd { return nil }
}

// fakeProxy keeps output ports in memory; a port forwards when its
// input is among inputs.
type fakeProxy struct {
	ports  []source.ProxyStatus
	inputs []source.Server
	fresh  int // fresh reads
}

func (f *fakeProxy) view() source.ProxyView {
	ports := slices.Clone(f.ports)
	for i, p := range ports {
		ports[i].State, ports[i].Server = source.ProxyIdle, nil
		if p.Input.None() {
			continue
		}
		ports[i].State = source.ProxyNoServer
		for _, sv := range f.inputs {
			if source.InputOf(sv) == p.Input {
				ports[i].State, ports[i].Server = source.ProxyForwarding, &sv
			}
		}
	}
	return source.ProxyView{Ports: ports, Inputs: f.inputs}
}

func (f *fakeProxy) View(_ context.Context, fresh bool) (source.ProxyView, error) {
	if fresh {
		f.fresh++
	}
	return f.view(), nil
}

func (f *fakeProxy) Add(_ context.Context, port int) (source.ProxyView, error) {
	if port < 1024 {
		return source.ProxyView{}, fmt.Errorf("Enter a port from 1024 to 65535.")
	}
	f.ports = append(f.ports, source.ProxyStatus{Port: port})
	return f.view(), nil
}

func (f *fakeProxy) Remove(_ context.Context, port int) (source.ProxyView, error) {
	f.ports = slices.DeleteFunc(f.ports, func(p source.ProxyStatus) bool { return p.Port == port })
	return f.view(), nil
}

func (f *fakeProxy) SetInput(_ context.Context, port int, in source.ProxyInput) (source.ProxyView, error) {
	for i := range f.ports {
		if f.ports[i].Port == port {
			f.ports[i].Input = in
		}
	}
	return f.view(), nil
}

var web = source.Server{Track: "t1", TrackName: "fix-login", Repo: "api", Name: "web", Type: "rspack", Port: 20013, State: "ready"}

func proxyTabModel(t *testing.T, f *fakeProxy) Model {
	t.Helper()
	m := New(Config{Version: "test", Theme: theme.Default(), Proxy: f})
	return settle(m, tea.WindowSizeMsg{Width: 120, Height: 40}, tea.KeyPressMsg{Code: tea.KeyTab})
}

func TestProxyTabMark(t *testing.T) {
	f := &fakeProxy{inputs: []source.Server{web}, ports: []source.ProxyStatus{{Port: 3000}}}
	m := proxyTabModel(t, f)
	if m.tab != tabProxy || strings.Contains(plainView(m), "Proxy ●") {
		t.Fatal("the Proxy tab is second, unmarked while nothing forwards")
	}
	f.ports[0].Input = source.InputOf(web)
	m = settle(m, proxyTickMsg{})
	if !strings.Contains(plainView(m), "Proxy ●") {
		t.Fatal("a port forwarding to a running server should mark the tab")
	}
	if m = clickText(t, m, "Repositories"); m.tab != tabRepositories {
		t.Errorf("clicking Repositories after the mark: tab %d", m.tab)
	}
}

func TestAddingAPort(t *testing.T) {
	f := &fakeProxy{}
	m := proxyTabModel(t, f)
	m = clickText(t, m, "Add new proxy port")
	if !m.proxy.adding || !m.proxy.input.Focused() {
		t.Fatal("Add new proxy port should open a new row with its input focused")
	}
	m = typeText(m, "8x0")
	m = settle(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.proxy.adding || !strings.Contains(plainView(m), "Enter a port from 1024") {
		t.Fatal("port 80 should be refused under the input, which stays")
	}
	m = settle(m, tea.KeyPressMsg{Code: tea.KeyEscape}, tea.KeyPressMsg{Code: 'n', Text: "n"})
	if m.proxy.input.Value() != "" {
		t.Errorf("Esc then n: input %q; want a fresh row", m.proxy.input.Value())
	}
	m = typeText(m, "3000")
	m = settle(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.proxy.adding || len(f.ports) != 1 || f.ports[0].Port != 3000 || m.proxy.selected != 0 {
		t.Fatalf("after Enter: adding %v, ports %+v, selected %d", m.proxy.adding, f.ports, m.proxy.selected)
	}
	view := plainView(m)
	if !strings.Contains(view, "[ 3000") || !strings.Contains(view, "[ none") {
		t.Error("the new port's row should show 3000 forwarding to none")
	}
}

func TestPickingAnInput(t *testing.T) {
	found := source.Server{Track: "t1", TrackName: "fix-login", Port: 5173, Type: "vite", State: "ready"}
	f := &fakeProxy{inputs: []source.Server{web, found}, ports: []source.ProxyStatus{{Port: 3000}, {Port: 8081}}}
	m := proxyTabModel(t, f)
	m = settle(m, tea.KeyPressMsg{Code: tea.KeyDown}, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.picker == nil || m.pickerFor != pickProxy || len(m.picker.Items) != 3 || m.picker.Marked != 0 {
		t.Fatalf("Enter on 8081: picker %+v; want none and the two servers, none marked", m.picker)
	}
	if it := m.picker.Items[2]; it.Label != ":5173" || it.Detail != "vite · fix-login" {
		t.Errorf("a found server's item = %+v", it)
	}
	m = settle(m, tea.KeyPressMsg{Code: tea.KeyDown}, tea.KeyPressMsg{Code: tea.KeyDown}, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.picker != nil || f.ports[1].Input != source.InputOf(found) {
		t.Fatalf("choosing :5173: ports %+v", f.ports)
	}
	if !strings.Contains(plainView(m), ":5173 · vite · fix-login") {
		t.Error("8081's row should show its input")
	}
	m = clickText(t, m, "[ none")
	if m.picker == nil || m.proxy.selected != 0 {
		t.Error("clicking 3000's input should open the picker for it")
	}
}

func TestRemovingAPort(t *testing.T) {
	f := &fakeProxy{ports: []source.ProxyStatus{{Port: 3000}, {Port: 8081}, {Port: 9000}}}
	m := proxyTabModel(t, f)
	m = settle(m, tea.KeyPressMsg{Code: tea.KeyDown}, tea.KeyPressMsg{Code: tea.KeyDelete})
	if len(f.ports) != 2 || f.ports[1].Port != 9000 {
		t.Fatalf("Delete on 8081: %+v", f.ports)
	}
	m = clickText(t, m, "×")
	if len(f.ports) != 1 || f.ports[0].Port != 9000 {
		t.Errorf("clicking the first ×: %+v; want 3000 gone", f.ports)
	}
}

func TestThePortsScroll(t *testing.T) {
	f := &fakeProxy{}
	for port := 3000; port < 3012; port++ {
		f.ports = append(f.ports, source.ProxyStatus{Port: port})
	}
	m := New(Config{Version: "test", Theme: theme.Default(), Proxy: f})
	m = settle(m, tea.WindowSizeMsg{Width: 120, Height: 30}, tea.KeyPressMsg{Code: tea.KeyTab})
	for range len(f.ports) {
		m = settle(m, tea.KeyPressMsg{Code: tea.KeyDown})
	}
	if m.proxy.offset == 0 || !strings.Contains(plainView(m), "Add new proxy port") {
		t.Fatalf("offset %d; the frame should scroll down to the add button", m.proxy.offset)
	}
	m = settle(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.proxy.adding || !strings.Contains(plainView(m), "Enter adds it") {
		t.Error("Enter on the add button should show the new row, scrolled into view")
	}
	m = settle(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	for range len(f.ports) {
		m = settle(m, tea.KeyPressMsg{Code: tea.KeyUp})
	}
	if m.proxy.offset != 0 {
		t.Errorf("back on the first port: offset %d", m.proxy.offset)
	}
}

func TestARefreshKeepsTheWheel(t *testing.T) {
	f := &fakeProxy{}
	for port := 3000; port < 3012; port++ {
		f.ports = append(f.ports, source.ProxyStatus{Port: port})
	}
	m := New(Config{Version: "test", Theme: theme.Default(), Proxy: f})
	m = settle(m, tea.WindowSizeMsg{Width: 120, Height: 30}, tea.KeyPressMsg{Code: tea.KeyTab})
	m = settle(m, tea.MouseWheelMsg{X: 10, Y: 20, Button: tea.MouseWheelDown})
	offset := m.proxy.offset
	if offset == 0 {
		t.Fatal("the wheel didn't scroll")
	}
	if m = settle(m, proxyTickMsg{}); m.proxy.offset != offset {
		t.Errorf("after a refresh: offset %d, want %d", m.proxy.offset, offset)
	}
}

func TestAReadFromBeforeAChangeIsDropped(t *testing.T) {
	f := &fakeProxy{ports: []source.ProxyStatus{{Port: 3000}, {Port: 8081}}}
	m := proxyTabModel(t, f)
	stale := proxyMsg{view: f.view(), fresh: true, seq: m.proxy.changes}
	m = settle(m, tea.KeyPressMsg{Code: tea.KeyDelete})
	if m = settle(m, stale); len(m.proxy.view.Ports) != 1 {
		t.Errorf("a read from before the removal brought back %+v", m.proxy.view.Ports)
	}
}

func TestThePickerSetsItsOwnPort(t *testing.T) {
	f := &fakeProxy{inputs: []source.Server{web}, ports: []source.ProxyStatus{{Port: 3000}, {Port: 8081}}}
	m := proxyTabModel(t, f)
	m = settle(m, tea.KeyPressMsg{Code: tea.KeyDown}, tea.KeyPressMsg{Code: tea.KeyEnter})
	// 3000 goes away while the picker for 8081 is open.
	f.ports = f.ports[1:]
	m = settle(m, proxyTickMsg{}, tea.KeyPressMsg{Code: tea.KeyDown}, tea.KeyPressMsg{Code: tea.KeyEnter})
	if len(f.ports) != 1 || f.ports[0].Port != 8081 || f.ports[0].Input != source.InputOf(web) {
		t.Errorf("ports %+v; want 8081 forwarding to web", f.ports)
	}
}
