package tracksview

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/commentstyle"
	"github.com/bluegardenproject/tracks/internal/theme"
)

// openPersonal opens Settings on Personalisation, learning with learn.
func openPersonal(t *testing.T, status commentstyle.Status, learn func(func(string)) (commentstyle.Status, error)) Model {
	t.Helper()
	m := New(Config{Version: "test", Theme: theme.Default(), Themes: newFakeThemes(), StylePath: "/cfg/comment-style.md",
		StyleStatus: func() (commentstyle.Status, error) { return status, nil }, LearnStyle: learn})
	m = settle(m, tea.WindowSizeMsg{Width: 140, Height: 44}, shiftTabKey)
	m = settle(m, m.loadStyleStatus()())
	return m.showSection(sectionPersonal)
}

func TestLearningTheCommentStyle(t *testing.T) {
	learned := commentstyle.Status{Learned: true, At: time.Date(2026, 10, 14, 9, 30, 0, 0, time.UTC), Comments: 212}
	var calls int
	m := openPersonal(t, commentstyle.Status{}, func(progress func(string)) (commentstyle.Status, error) {
		calls++
		progress(commentstyle.Line{Kind: commentstyle.Request, Text: "GET user"}.String())
		progress(commentstyle.Line{Kind: commentstyle.Answer, Text: "you're me"}.String())
		progress(commentstyle.Line{Kind: commentstyle.Done, Text: "saved /cfg/comment-style.md"}.String())
		return learned, nil
	})
	view := plainView(m)
	for _, want := range []string{"Personalisation", "PR comment style", "Not learned yet.", "Learn my comment style", "log", "idle", "Ready."} {
		if !strings.Contains(view, want) {
			t.Errorf("before learning, the section lacks %q", want)
		}
	}

	m = settle(m, enterKey) // into the section
	m = settle(m, enterKey) // learn
	if calls != 1 {
		t.Fatalf("Enter learned %d times, want once", calls)
	}
	view = plainView(m)
	for _, want := range []string{"→ GET user", "← you're me", "✓ saved /cfg/comment-style.md", "✓ done", "from 212 comments", "Relearn"} {
		if !strings.Contains(view, want) {
			t.Errorf("after learning, the section lacks %q", want)
		}
	}
}

func TestLearningFails(t *testing.T) {
	m := openPersonal(t, commentstyle.Status{}, func(func(string)) (commentstyle.Status, error) {
		return commentstyle.Status{}, errors.New("gh isn't installed")
	})
	m = settle(m, enterKey, enterKey)
	view := plainView(m)
	if !strings.Contains(view, "✗ gh isn't installed") || !strings.Contains(view, "✗ failed") {
		t.Errorf("a failure should end the log and mark it failed:\n%s", view)
	}
	if !strings.Contains(view, "Learn my comment style") {
		t.Error("after a failure, the button should still offer to learn")
	}
}

func TestClickingLearn(t *testing.T) {
	var calls int
	m := openPersonal(t, commentstyle.Status{Learned: true}, func(func(string)) (commentstyle.Status, error) {
		calls++
		return commentstyle.Status{Learned: true}, nil
	})
	if !strings.Contains(plainView(m), "Written by hand.") {
		t.Error("a guide without a header should read as written by hand")
	}
	p := m.settingsPanes()
	_, row := m.personalTop(m.sectionWidth())
	m = settle(m, tea.MouseClickMsg{X: p.secX + 3, Y: m.bodyTop() + row, Button: tea.MouseLeft})
	if calls != 1 {
		t.Errorf("clicking Relearn learned %d times, want once", calls)
	}
}
