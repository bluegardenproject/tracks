package dashboard

import (
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/daemon"
	tea "github.com/charmbracelet/bubbletea"
)

// sortedProxies orders defined ports by public port, stable frame to frame.
func TestSortedProxiesByPort(t *testing.T) {
	got := sortedProxies([]daemon.ProxyEntryStatus{
		{PublicPort: 8081},
		{PublicPort: 3000},
	})
	if got[0].PublicPort != 3000 || got[1].PublicPort != 8081 {
		t.Fatalf("not sorted by port: %+v", got)
	}
}

// A free port renders "(free)"; a linked port names its track/service.
func TestRenderProxyPortShowsUpstream(t *testing.T) {
	m := &model{styles: defaultStyles()}
	free := m.renderProxyPort(-1, daemon.ProxyEntryStatus{PublicPort: 3000})
	if !strings.Contains(free, ":3000") || !strings.Contains(free, "free") {
		t.Errorf("free port row = %q", free)
	}
	linked := m.renderProxyPort(-1, daemon.ProxyEntryStatus{
		PublicPort:    3000,
		Upstream:      "localhost:24010",
		ActiveTrackID: "20260101-000000-bbbbbb",
		ActiveService: "dev",
	})
	if !strings.Contains(linked, "dev") {
		t.Errorf("linked port row missing service: %q", linked)
	}
}

// The Proxy view must obey the same height budget as the tracks view.
func TestProxyViewNeverExceedsHeight(t *testing.T) {
	m := &model{
		styles: defaultStyles(),
		mode:   modeProxy,
		width:  100,
		proxies: []daemon.ProxyEntryStatus{
			{PublicPort: 3000, Upstream: "localhost:24010", ActiveTrackID: "20260101-000000-aaaaaa", ActiveService: "dev"},
			{PublicPort: 8081},
		},
	}
	for _, h := range []int{8, 12, 20, 40} {
		m.height = h
		got := lineCount(m.View())
		if got > h {
			t.Errorf("height %d: view rendered %d lines", h, got)
		}
	}
}

// Tab toggles between the two views.
func TestTabTogglesMode(t *testing.T) {
	m := &model{styles: defaultStyles()}
	if _, _ = m.Update(keyMsg("tab")); m.mode != modeProxy {
		t.Fatalf("first tab should switch to proxy mode, got %v", m.mode)
	}
	if _, _ = m.Update(keyMsg("tab")); m.mode != modeTracks {
		t.Fatalf("second tab should switch back to tracks mode, got %v", m.mode)
	}
}

func keyMsg(s string) tea.KeyMsg {
	if s == "tab" {
		return tea.KeyMsg{Type: tea.KeyTab}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// With more ports than fit, the Proxy view must still keep the selected row
// on screen and the footer hints visible (scrolling window).
func TestProxyViewKeepsCursorAndFooterVisible(t *testing.T) {
	proxies := make([]daemon.ProxyEntryStatus, 30)
	for i := range proxies {
		proxies[i] = daemon.ProxyEntryStatus{PublicPort: 3000 + i}
	}
	m := &model{
		styles:      defaultStyles(),
		mode:        modeProxy,
		width:       120,
		height:      14,
		proxies:     proxies,
		proxyCursor: 25, // sorted by port, this is :3025
	}
	out := m.View()
	if lineCount(out) > m.height {
		t.Fatalf("view exceeded height: %d > %d", lineCount(out), m.height)
	}
	if !strings.Contains(out, ":3025") {
		t.Error("selected row :3025 scrolled off screen")
	}
	if !strings.Contains(out, "tab tracks") {
		t.Error("footer hints were clipped away")
	}
}

// Guard against accidentally dropping the proxy footer hints.
func TestProxyViewShowsKeyHints(t *testing.T) {
	m := &model{styles: defaultStyles(), mode: modeProxy, width: 120, height: 30}
	out := m.View()
	for _, want := range []string{"add, remove, link", "tab tracks"} {
		if !strings.Contains(out, want) {
			t.Errorf("proxy view missing hint %q", want)
		}
	}
}
