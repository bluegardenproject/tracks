package dashboard

import (
	"fmt"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/daemon"
	"github.com/bluegardenproject/tracks/internal/state"
	tea "github.com/charmbracelet/bubbletea"
)

func liveService(name string, port int) state.ServiceState {
	return state.ServiceState{Name: name, Status: state.ServiceRunning, Port: port}
}

// proxyRows should flatten only *live* services across tracks and annotate
// each with its proxy's fixed port and whether it's the active upstream.
func TestProxyRowsFlattensLiveServicesAndAnnotates(t *testing.T) {
	m := &model{
		styles: defaultStyles(),
		tracks: []state.Track{
			{
				ID:   "20260101-000000-aaaaaa",
				Slug: "alpha",
				Services: []state.ServiceState{
					liveService("metro", 24010),
					{Name: "old", Status: state.ServiceStopped, Port: 24011}, // not live
				},
			},
			{
				ID:   "20260101-000000-bbbbbb",
				Slug: "beta",
				Services: []state.ServiceState{
					liveService("metro", 24060),
				},
			},
		},
		proxies: []daemon.ProxyEntryStatus{
			{ServiceName: "metro", PublicPort: 8081, Upstream: "localhost:24060", ActiveTrackID: "20260101-000000-bbbbbb"},
		},
	}

	rows := m.proxyRows()
	if len(rows) != 2 {
		t.Fatalf("expected 2 live rows (stopped service excluded), got %d", len(rows))
	}
	// Sorted by service then trackLabel: alpha before beta.
	if rows[0].trackLabel != "alpha" || rows[1].trackLabel != "beta" {
		t.Fatalf("rows not sorted by track label: %+v", rows)
	}
	for _, r := range rows {
		if r.proxyPort != 8081 {
			t.Errorf("%s: proxyPort = %d, want 8081", r.trackLabel, r.proxyPort)
		}
	}
	if rows[0].active {
		t.Error("alpha should not be the active upstream")
	}
	if !rows[1].active {
		t.Error("beta should be the active upstream")
	}
}

// A service with no configured proxy_port shows proxyPort 0 and is not linkable.
func TestProxyRowsNoProxyConfigured(t *testing.T) {
	m := &model{
		styles: defaultStyles(),
		tracks: []state.Track{{
			ID:       "20260101-000000-cccccc",
			Services: []state.ServiceState{liveService("web", 3001)},
		}},
	}
	rows := m.proxyRows()
	if len(rows) != 1 || rows[0].proxyPort != 0 {
		t.Fatalf("expected one row with proxyPort 0, got %+v", rows)
	}
}

// The Proxy view must obey the same height budget as the tracks view.
func TestProxyViewNeverExceedsHeight(t *testing.T) {
	m := &model{
		styles: defaultStyles(),
		mode:   modeProxy,
		width:  100,
		tracks: []state.Track{{
			ID:       "20260101-000000-dddddd",
			Slug:     "gamma",
			Services: []state.ServiceState{liveService("metro", 24010), liveService("api", 24011)},
		}},
		proxies: []daemon.ProxyEntryStatus{{ServiceName: "metro", PublicPort: 8081}},
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

// With more running servers than fit, the Proxy view must still keep the
// selected row on screen and the footer hints visible (scrolling window).
func TestProxyViewKeepsCursorAndFooterVisible(t *testing.T) {
	tracks := make([]state.Track, 30)
	for i := range tracks {
		tracks[i] = state.Track{
			ID:       fmt.Sprintf("20260101-000000-t%05d", i),
			Slug:     fmt.Sprintf("srv%02d", i),
			Services: []state.ServiceState{liveService("metro", 24000+i)},
		}
	}
	m := &model{
		styles:      defaultStyles(),
		mode:        modeProxy,
		width:       120,
		height:      14,
		tracks:      tracks,
		proxyCursor: 25, // rows are sorted by slug, so this is "srv25"
	}
	out := m.View()
	if lineCount(out) > m.height {
		t.Fatalf("view exceeded height: %d > %d", lineCount(out), m.height)
	}
	if !strings.Contains(out, "srv25") {
		t.Error("selected row srv25 scrolled off screen")
	}
	if !strings.Contains(out, "tab tracks") {
		t.Error("footer hints were clipped away")
	}
}

// Guard against accidentally dropping the proxy footer hints.
func TestProxyViewShowsKeyHints(t *testing.T) {
	m := &model{styles: defaultStyles(), mode: modeProxy, width: 120, height: 30}
	out := m.View()
	for _, want := range []string{"link to proxy", "free port", "stop server", "tab tracks"} {
		if !strings.Contains(out, want) {
			t.Errorf("proxy view missing hint %q", want)
		}
	}
}
