package tracksview

import (
	"context"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
	"github.com/bluegardenproject/tracks/internal/v2/ui/source"
)

type fakeRepos struct {
	entries []source.RepositoryEntry
	nextID  int64
	deleted []int64
}

func (f *fakeRepos) List(context.Context) ([]source.RepositoryEntry, error) {
	return slices.Clone(f.entries), nil
}

func (f *fakeRepos) Suggest(_ context.Context, path string) (source.RepositoryEntry, error) {
	if !strings.HasPrefix(path, "/") {
		return source.RepositoryEntry{}, &source.FieldError{Field: "path", Message: "Use an absolute path."}
	}
	return source.RepositoryEntry{
		Repo:   source.Repository{Path: path, Name: "suggested", BaseBranch: "trunk"},
		Remote: "https://github.com/acme/shop",
	}, nil
}

func (f *fakeRepos) Add(_ context.Context, r source.Repository) (source.Repository, error) {
	if r.Name == "taken" {
		return source.Repository{}, &source.FieldError{Field: "name", Message: "Another repo already has this name."}
	}
	f.nextID++
	r.ID = f.nextID
	f.entries = append(f.entries, source.RepositoryEntry{Repo: r})
	return r, nil
}

func (f *fakeRepos) Update(_ context.Context, r source.Repository) (source.Repository, error) {
	for i := range f.entries {
		if f.entries[i].ID == r.ID {
			f.entries[i].Repo = r
		}
	}
	return r, nil
}

func (f *fakeRepos) Delete(_ context.Context, id int64) error {
	f.deleted = append(f.deleted, id)
	f.entries = slices.DeleteFunc(f.entries, func(e source.RepositoryEntry) bool { return e.ID == id })
	return nil
}

// settle runs msg and every command it leads to.
func settle(m Model, msgs ...tea.Msg) Model {
	for len(msgs) > 0 {
		msg := msgs[0]
		msgs = msgs[1:]
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, c := range batch {
				if c != nil {
					msgs = append(msgs, c())
				}
			}
			continue
		}
		next, cmd := m.Update(msg)
		m = next.(Model)
		if cmd != nil {
			if out := cmd(); out != nil {
				msgs = append(msgs, out)
			}
		}
	}
	return m
}

func reposTab(t *testing.T, f *fakeRepos) Model {
	t.Helper()
	m := New(Config{Version: "test", Theme: theme.Default(), Repos: f})
	return settle(m, tea.WindowSizeMsg{Width: 120, Height: 40}, tea.KeyPressMsg{Code: tea.KeyTab})
}

func typeText(m Model, s string) Model {
	for _, r := range s {
		m = settle(m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

// press clicks the form button showing label and runs what follows.
func press(t *testing.T, m Model, label string) Model {
	t.Helper()
	for y, line := range strings.Split(plainView(m), "\n") {
		if x := strings.Index(line, " "+label+" "); x >= 0 && strings.Contains(line[:x], "│") {
			return settle(m, tea.MouseClickMsg{X: len([]rune(line[:x])) + 1, Y: y, Button: tea.MouseLeft})
		}
	}
	t.Fatalf("button %s not drawn", label)
	return m
}

func plainView(m Model) string { return escapes.ReplaceAllString(m.View().Content, "") }

func TestAddingARepo(t *testing.T) {
	f := &fakeRepos{}
	m := reposTab(t, f)
	if m.tab != tabRepositories || !strings.Contains(plainView(m), "No repositories yet") {
		t.Fatal("the Repositories tab should show that there are no repos")
	}
	m = settle(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.repos.editing || m.repos.form.focus != fieldName {
		t.Fatalf("Enter on New: editing %v, focus %d; want the name field", m.repos.editing, m.repos.form.focus)
	}
	m = settle(m, tea.KeyPressMsg{Code: tea.KeyTab})
	m = typeText(m, "/src/shop")
	m = settle(m, tea.KeyPressMsg{Code: tea.KeyTab})
	if m.tab != tabRepositories || m.repos.form.value(fieldName) != "suggested" || m.repos.form.value(fieldBase) != "trunk" {
		t.Fatalf("leaving the path: tab %d, name %q, base %q; want the suggestion filled in",
			m.tab, m.repos.form.value(fieldName), m.repos.form.value(fieldBase))
	}
	view := plainView(m)
	if !strings.Contains(view, "https://github.com/acme/shop") || !strings.Contains(view, "Options") {
		t.Error("the form should show the path's remote and the Options section")
	}
	if !strings.Contains(plainView(m), " Save ") {
		t.Fatal("a filled-in new repo should show Save")
	}
	m = press(t, m, "Save")
	if len(f.entries) != 1 || f.entries[0].Path != "/src/shop" || m.repos.editing {
		t.Fatalf("after Save: %+v, editing %v; want the repo added and focus back on the list", f.entries, m.repos.editing)
	}
	if m.repos.selected != 0 || m.repos.notice.text != "Saved suggested." {
		t.Errorf("after Save: selected %d, notice %q; want the new repo selected", m.repos.selected, m.repos.notice.text)
	}
}

func TestFieldErrorsStayInTheForm(t *testing.T) {
	m := reposTab(t, &fakeRepos{})
	m = settle(m, tea.KeyPressMsg{Code: 'n', Text: "n"})
	m = typeText(m, "taken")
	m = settle(m, tea.KeyPressMsg{Code: tea.KeyTab})
	m = typeText(m, "relative")
	m = settle(m, tea.KeyPressMsg{Code: tea.KeyTab})
	if m.repos.form.focus != fieldBase || !strings.Contains(plainView(m), "Use an absolute path.") {
		t.Error("a bad path should explain itself under the field")
	}
	m = settle(m, tea.KeyPressMsg{Code: tea.KeyTab}, tea.KeyPressMsg{Code: tea.KeyTab})
	if m.repos.form.focus != fieldSave {
		t.Fatalf("focus %d, want Save", m.repos.form.focus)
	}
	m = settle(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.repos.editing || m.repos.form.focus != fieldName || !strings.Contains(plainView(m), "Another repo already") {
		t.Errorf("a taken name should keep the form open on the name field with the message")
	}
}

func TestUnsavedChangesAsk(t *testing.T) {
	f := &fakeRepos{}
	for _, name := range []string{"api", "web"} {
		_, _ = f.Add(context.Background(), source.Repository{Name: name, Path: "/src/" + name, BaseBranch: "main"})
	}
	m := reposTab(t, f)
	m = settle(m, tea.KeyPressMsg{Code: tea.KeyDown})
	if m.repos.selected != 0 || strings.Contains(plainView(m), " Save ") {
		t.Fatalf("selecting api: selected %d; Save should only show after a change", m.repos.selected)
	}
	m = settle(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m = typeText(m, "-2")

	m = settle(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.repos.leaving == nil || !strings.Contains(plainView(m), "Unsaved changes.") {
		t.Fatal("Esc with changes should ask")
	}
	m = press(t, m, "Cancel")
	if m.repos.leaving != nil || !m.repos.editing || m.repos.form.value(fieldName) != "api-2" {
		t.Fatal("Cancel should keep editing with the changes")
	}

	m = clickLabel(t, m, "Settings")
	if m.tab != tabRepositories || m.repos.leaving == nil {
		t.Fatal("switching tabs with changes should ask first")
	}
	m = press(t, m, "Discard")
	if m.tab != tabSettings || f.entries[0].Name != "api" {
		t.Fatalf("Discard: tab %d, stored %q; want Settings and api unchanged", m.tab, f.entries[0].Name)
	}

	m = settle(m, tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}, tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}, tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	m = settle(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m = typeText(m, "-2")
	m = settle(m, tea.KeyPressMsg{Code: tea.KeyEscape}, tea.KeyPressMsg{Code: 's', Text: "s"})
	if f.entries[0].Name != "api-2" || m.repos.editing {
		t.Errorf("Save from the question: stored %q, editing %v; want api-2 saved and the list focused", f.entries[0].Name, m.repos.editing)
	}
}

func TestDeletingAsksFirst(t *testing.T) {
	f := &fakeRepos{}
	_, _ = f.Add(context.Background(), source.Repository{Name: "api", Path: "/src/api", BaseBranch: "main"})
	f.entries[0].Tracks = []string{"api-auth", "fix-login"}
	m := reposTab(t, f)
	m = settle(m, tea.KeyPressMsg{Code: tea.KeyDown})
	if !strings.Contains(plainView(m), "2  api-auth, fix-login") {
		t.Error("the details should count and name the active tracks")
	}
	f.entries[0].Tracks = nil
	m = settle(m, m.loadRepos()())
	m = press(t, m, "Delete")
	m = press(t, m, "Cancel")
	if len(f.deleted) != 0 {
		t.Fatal("Cancel deleted the repo")
	}
	m = press(t, m, "Delete")
	m = press(t, m, "Delete repository")
	if len(f.deleted) != 1 || len(m.repos.entries) != 0 || m.repos.selected != -1 {
		t.Errorf("deleted %v, %d left, selected %d; want api gone and New selected", f.deleted, len(m.repos.entries), m.repos.selected)
	}
}

func TestButtonsHover(t *testing.T) {
	m := reposTab(t, &fakeRepos{})
	x, y := -1, -1
	for row, line := range strings.Split(plainView(m), "\n") {
		if i := strings.Index(line, newButton.Label); i >= 0 {
			x, y = len([]rune(line[:i])), row
			break
		}
	}
	if m = settle(m, tea.MouseMotionMsg{X: x, Y: y}); !m.repos.hoverNew {
		t.Fatal("the mouse on New should highlight it")
	}
	if m = settle(m, tea.MouseMotionMsg{X: 0, Y: 0}); m.repos.hoverNew || m.repos.hoverField != -1 {
		t.Error("moving away should clear the highlight")
	}
}
