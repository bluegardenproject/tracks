package tracksview

import (
	"context"
	"maps"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/v2/agents"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
	"github.com/bluegardenproject/tracks/internal/v2/ui/widget"
)

const (
	mcpButton = "Check MCP servers"
	mcpHint   = "The CLI connects to every server to check it, so this can take a while. Only servers set up for every project are listed; a repo can add its own."
	mcpScope  = "Only servers set up for every project; a repo can add its own."
)

// checkMCP asks id's CLI for its MCP servers, looking for the CLI first
// if the last look didn't find it.
func (m Model) checkMCP(id string) (Model, tea.Cmd) {
	en, ok := agents.ByID(id)
	if !ok || m.engineSource == nil {
		return m, nil
	}
	m.engines.mcp = maps.Clone(m.engines.mcp)
	m.engines.mcp[id] = mcpList{checking: true}
	src, path := m.engineSource, m.engines.checks[id].found.Path
	return m, func() tea.Msg {
		if path == "" {
			found, err := src.Check(context.Background(), en)
			if err != nil {
				return engineMCPMsg{id: id, err: err}
			}
			path = found.Path
		}
		servers, err := src.MCP(context.Background(), path)
		return engineMCPMsg{id, servers, err}
	}
}

func (m Model) engineMCP(msg engineMCPMsg) Model {
	if m.engines.settings.Get(msg.id) == nil {
		return m
	}
	m.engines.mcp = maps.Clone(m.engines.mcp)
	m.engines.mcp[msg.id] = mcpList{checked: true, servers: msg.servers, err: msg.err}
	return m
}

var mcpColors = map[agents.MCPState]theme.Token{
	agents.MCPReady:     theme.StateSuccessText,
	agents.MCPAttention: theme.StateWarningText,
	agents.MCPFailed:    theme.StateDangerText,
}

// mcp lists the engine's MCP servers once checked, with the button
// that checks them.
func (b *engineBody) mcp(en agents.Engine, width int) {
	m := b.m
	l := m.engines.mcp[en.ID]
	label := b.label("MCP servers")
	button := widget.NewButton(mcpButton, widget.ButtonDefault)
	rest := width - engineLabelWidth
	switch {
	case l.checking:
		b.add(label + m.fg(theme.TextMuted).Render("Connecting to each server…"))
		return
	case l.err != nil:
		b.buttons(label, engineLabelWidth, []int{ctlMCP}, button)
		b.wrapped(theme.StateDangerText, "Couldn't check them: "+checkProblem(en, l.err), rest)
		return
	case !l.checked:
		b.buttons(label, engineLabelWidth, []int{ctlMCP}, button)
		b.wrapped(theme.TextFaint, mcpHint, rest)
		b.mcpNote(en, rest)
		return
	}
	if len(l.servers) == 0 {
		b.add(label + m.fg(theme.TextFaint).Render(cut("None are set up for every project.", rest)))
	}
	nameWidth, statusWidth := 0, 0
	for _, s := range l.servers {
		nameWidth = max(nameWidth, len([]rune(s.Name))+2)
		statusWidth = max(statusWidth, len([]rune(s.Status))+2)
	}
	statusWidth = min(statusWidth, max(0, rest-nameWidth))
	for i, s := range l.servers {
		prefix := b.label("")
		if i == 0 {
			prefix = label
		}
		line := m.fg(theme.TextDefault).Render(pad(cut(s.Name, nameWidth), nameWidth)) +
			m.fg(mcpColors[s.State]).Render(pad(cut(s.Status, statusWidth-2), statusWidth)) +
			m.fg(theme.TextFaint).Render(cut(s.Detail, max(0, rest-nameWidth-statusWidth)))
		b.add(prefix + line)
	}
	b.buttons(b.label(""), engineLabelWidth, []int{ctlMCP}, button)
	b.wrapped(theme.TextFaint, mcpScope, rest)
	b.mcpNote(en, rest)
}

// mcpNote says what en's `mcp list` leaves out.
func (b *engineBody) mcpNote(en agents.Engine, width int) {
	if en.MCPNote != "" {
		b.wrapped(theme.TextFaint, en.MCPNote, width)
	}
}
