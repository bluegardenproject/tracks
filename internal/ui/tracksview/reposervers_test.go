package tracksview

import (
	"context"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/ui/source"
)

func TestSetupAndServersStartAsButtons(t *testing.T) {
	m := reposTab(t, &fakeRepos{})
	m = settle(m, tea.KeyPressMsg{Code: 'n', Text: "n"})
	view := plainView(m)
	if !strings.Contains(view, " Add setup ") || !strings.Contains(view, " Add servers ") {
		t.Fatal("a new repo should offer Add setup and Add servers")
	}
	if strings.Contains(view, "Dev servers") || strings.Contains(view, "Remove setup") {
		t.Error("the sections should stay hidden until added")
	}
}

func TestAddingSetupAndServers(t *testing.T) {
	f := &fakeRepos{}
	_, _ = f.Add(context.Background(), source.Repository{Name: "shop", Path: "/src/shop", BaseBranch: "main"})
	m := reposTab(t, f)
	m = settle(m, tea.KeyPressMsg{Code: tea.KeyDown}, tea.KeyPressMsg{Code: tea.KeyEnter})

	m = press(t, m, "Add setup")
	if m.repos.form.focus != fieldSetup || strings.Contains(plainView(m), " Add setup ") {
		t.Fatalf("Add setup: focus %d; want the command focused and the button gone", m.repos.form.focus)
	}
	m = typeText(m, "pnpm install")

	m = press(t, m, "Add servers")
	if m.repos.form.focus != serverField(0, serverName) || strings.Contains(plainView(m), " Add servers ") {
		t.Fatalf("Add servers: focus %d; want the first server's name", m.repos.form.focus)
	}
	m = typeText(m, "web")
	m = settle(m, tea.KeyPressMsg{Code: tea.KeyTab})
	m = typeText(m, "pnpm dev --port $PORT")
	m = settle(m, tea.KeyPressMsg{Code: tea.KeyTab}, tea.KeyPressMsg{Code: tea.KeyTab})
	if m.repos.form.focus != serverField(0, serverMode) {
		t.Fatalf("focus %d, want the port mode", m.repos.form.focus)
	}
	if slices.Contains(m.repos.form.order(), serverField(0, serverPort)) {
		t.Error("the port number should only show for a fixed port")
	}
	m = settle(m, tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}, tea.KeyPressMsg{Code: tea.KeyTab})
	if m.repos.form.focus != serverField(0, serverPort) {
		t.Fatalf("after picking fixed: focus %d, want the port number", m.repos.form.focus)
	}
	m = typeText(m, "8081")
	m = settle(m, tea.KeyPressMsg{Code: tea.KeyTab})
	m = typeText(m, "metro")

	for m.repos.form.focus != fieldAddServer {
		m = settle(m, tea.KeyPressMsg{Code: tea.KeyTab})
	}
	m = settle(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m = typeText(m, "storybook")
	m, _ = m.pressField(serverField(0, serverRemove))
	if len(m.repos.form.servers) != 1 {
		t.Fatalf("%d servers after removing the first; want 1", len(m.repos.form.servers))
	}

	m = press(t, m, "Save")
	got := f.entries[0].Repo
	want := []source.DevServer{{Name: "storybook", PortMode: source.PortAssigned}}
	if got.Setup != "pnpm install" || !slices.Equal(got.Servers, want) {
		t.Errorf("saved setup %q, servers %+v; want pnpm install and storybook", got.Setup, got.Servers)
	}
}

func TestRemovingTheSetupBringsTheButtonBack(t *testing.T) {
	f := &fakeRepos{}
	_, _ = f.Add(context.Background(), source.Repository{Name: "shop", Path: "/src/shop", BaseBranch: "main", Setup: "pnpm install",
		Servers: []source.DevServer{{Name: "web", Command: "pnpm dev", PortMode: source.PortFixed, Port: 3000}}})
	m := reposTab(t, f)
	m = settle(m, tea.KeyPressMsg{Code: tea.KeyDown})
	view := plainView(m)
	if !strings.Contains(view, "Remove setup") || !strings.Contains(view, "Server 1 · web") || strings.Contains(view, " Add setup ") {
		t.Fatal("a repo with a setup and a server should show both sections")
	}
	if strings.Contains(view, " Save ") {
		t.Error("an unchanged repo with servers counts as changed")
	}
	m = press(t, m, "Remove setup")
	if !strings.Contains(plainView(m), " Add setup ") || !m.repos.form.dirty() {
		t.Error("removing the setup should bring Add setup back and count as a change")
	}
}

func TestServerErrorsFocusTheirField(t *testing.T) {
	f := &fakeRepos{}
	_, _ = f.Add(context.Background(), source.Repository{Name: "shop", Path: "/src/shop", BaseBranch: "main",
		Servers: []source.DevServer{{Name: "web", Command: "pnpm dev", PortMode: source.PortAssigned}, {Name: "api", Command: "go run .", PortMode: source.PortAssigned}}})
	m := reposTab(t, f)
	m = settle(m, tea.KeyPressMsg{Code: tea.KeyDown})
	if !m.repos.form.fieldError(&source.FieldError{Field: source.ServerField(1, "command"), Message: "Enter the command."}) {
		t.Fatal("a server field error wasn't taken")
	}
	if m.repos.form.focus != serverField(1, serverCommand) {
		t.Errorf("focus %d; want the second server's command", m.repos.form.focus)
	}
	m.repos.form.fieldError(&source.FieldError{Field: source.ServerField(0, "port"), Message: "Pick a mode."})
	if m.repos.form.focus != serverField(0, serverMode) {
		t.Errorf("a port error on an assigned port: focus %d; want the port mode", m.repos.form.focus)
	}
}

func TestTheFormScrollsToTheFocus(t *testing.T) {
	var servers []source.DevServer
	for _, name := range []string{"a", "b", "c", "d"} {
		servers = append(servers, source.DevServer{Name: name, Command: "run", PortMode: source.PortAssigned})
	}
	f := &fakeRepos{}
	_, _ = f.Add(context.Background(), source.Repository{Name: "shop", Path: "/src/shop", BaseBranch: "main", Servers: servers})
	m := reposTab(t, f)
	m = settle(m, tea.KeyPressMsg{Code: tea.KeyDown}, tea.KeyPressMsg{Code: tea.KeyEnter})
	for m.repos.form.focus != fieldAddServer {
		m = settle(m, tea.KeyPressMsg{Code: tea.KeyTab})
	}
	if m.repos.formOffset == 0 || !strings.Contains(plainView(m), " Add server ") {
		t.Errorf("offset %d; the form should scroll to Add server", m.repos.formOffset)
	}
	m = press(t, m, "Add server")
	if m.repos.form.focus != serverField(4, serverName) {
		t.Errorf("clicking the scrolled Add server: focus %d", m.repos.form.focus)
	}
}

func TestAnEmptySetupCantBeSaved(t *testing.T) {
	f := &fakeRepos{}
	_, _ = f.Add(context.Background(), source.Repository{Name: "shop", Path: "/src/shop", BaseBranch: "main"})
	m := reposTab(t, f)
	m = settle(m, tea.KeyPressMsg{Code: tea.KeyDown}, tea.KeyPressMsg{Code: tea.KeyEnter})
	m = press(t, m, "Add setup")
	m = settle(m, tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}, tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	for m.repos.form.focus != fieldSave {
		m = settle(m, tea.KeyPressMsg{Code: tea.KeyTab})
	}
	m = settle(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if f.entries[0].DraftPRs || m.repos.form.focus != fieldSetup || !strings.Contains(plainView(m), "Enter the setup command") {
		t.Error("saving with an empty setup should stay in the form and point at the command")
	}
}

func TestTheWheelScrollsTheFormItIsOver(t *testing.T) {
	var servers []source.DevServer
	for _, name := range []string{"a", "b", "c"} {
		servers = append(servers, source.DevServer{Name: name, Command: "run", PortMode: source.PortAssigned})
	}
	f := &fakeRepos{}
	_, _ = f.Add(context.Background(), source.Repository{Name: "shop", Path: "/src/shop", BaseBranch: "main", Servers: servers})
	_, _ = f.Add(context.Background(), source.Repository{Name: "web", Path: "/src/web", BaseBranch: "main"})
	m := reposTab(t, f)
	m = settle(m, tea.KeyPressMsg{Code: tea.KeyDown})
	rp := m.repoPanes()
	m = settle(m, tea.MouseWheelMsg{X: rp.formX + 4, Y: m.contentTop() + 4, Button: tea.MouseWheelDown})
	if m.repos.formOffset == 0 || m.repos.selected != 0 {
		t.Errorf("wheel over the form: offset %d, selected %d; want the form scrolled, shop still selected", m.repos.formOffset, m.repos.selected)
	}
	m = settle(m, tea.MouseWheelMsg{X: rp.listX + 1, Y: m.contentTop() + 4, Button: tea.MouseWheelDown})
	if m.repos.selected != 1 {
		t.Errorf("wheel over the list: selected %d, want web", m.repos.selected)
	}
}
