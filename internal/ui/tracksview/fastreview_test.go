package tracksview

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/theme"
	"github.com/bluegardenproject/tracks/internal/ui/widget"
)

func TestFastReview(t *testing.T) {
	var links []string
	m := New(Config{Version: "test", Theme: theme.Default(), FastReview: func(link string, progress func(string)) (string, error) {
		links = append(links, link)
		progress("Reading the pull request's title from GitHub…")
		return "[Fast Review] Fix the cache", nil
	}})
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40}, tracksMsg{tracks: demoTracks(3)})
	if !m.fastShown() {
		t.Fatal("no Fast Track frame in a 120×40 window")
	}

	m = settle(m, tea.PasteMsg{Content: " https://github.com/acme/web/pull/42\n"})
	if !m.station.fast.Focused() || m.station.fast.Value() != "https://github.com/acme/web/pull/42" {
		t.Fatalf("a paste on Station: focused %v, value %q", m.station.fast.Focused(), m.station.fast.Value())
	}
	m = settle(m, tea.KeyPressMsg{Code: 'j', Text: "j"})
	if m.station.selected != 0 || m.station.fast.Value() != "https://github.com/acme/web/pull/42j" {
		t.Errorf("j with the input focused: selected %d, value %q", m.station.selected, m.station.fast.Value())
	}
	m = settle(m, tea.KeyPressMsg{Code: tea.KeyBackspace})
	m = settle(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if len(links) != 1 || links[0] != "https://github.com/acme/web/pull/42" {
		t.Errorf("Enter opened %q", links)
	}
	if m.station.fast.Focused() || m.station.fast.Value() != "" || m.station.notice.text != "Opened [Fast Review] Fix the cache." {
		t.Errorf("after Enter: focused %v, value %q, notice %q", m.station.fast.Focused(), m.station.fast.Value(), m.station.notice.text)
	}

	m = settle(m, tea.KeyPressMsg{Code: 'f'})
	if !m.station.fast.Focused() {
		t.Error("f didn't focus the input")
	}
	if m = settle(m, tea.KeyPressMsg{Code: tea.KeyEscape}); m.station.fast.Focused() {
		t.Error("Esc didn't leave the input")
	}
	p := m.panes()
	if m = settle(m, tea.MouseClickMsg{X: p.detailsX + 4, Y: m.contentTop() + 1, Button: tea.MouseLeft}); !m.station.fast.Focused() {
		t.Error("a click on the input didn't focus it")
	}
	if m = settle(m, tea.MouseClickMsg{X: 10, Y: m.stationTop(), Button: tea.MouseLeft}); m.station.fast.Focused() {
		t.Error("a click on the list didn't leave the input")
	}

	inputW, _ := fastBoxes(p.detailsWidth - 4)
	button := tea.MouseClickMsg{X: p.detailsX + 2 + inputW + widget.ButtonGap + 1, Y: m.contentTop() + 1, Button: tea.MouseLeft}
	if m = settle(m, button); len(links) != 1 {
		t.Errorf("Review without a link opened %q", links)
	}
	m = settle(m, tea.PasteMsg{Content: "https://github.com/acme/web/pull/43"})
	if m = settle(m, button); len(links) != 2 || links[1] != "https://github.com/acme/web/pull/43" {
		t.Errorf("the Review button opened %q", links)
	}
}

func TestFastReviewWithoutDaemon(t *testing.T) {
	m := update(New(Config{Version: "test", Theme: theme.Default()}), tea.WindowSizeMsg{Width: 120, Height: 40},
		tracksMsg{tracks: demoTracks(3)})
	if m = settle(m, tea.PasteMsg{Content: "https://github.com/acme/web/pull/42"}); m.station.fast.Focused() {
		t.Error("a paste focused Fast Review without a daemon to open it")
	}
}
