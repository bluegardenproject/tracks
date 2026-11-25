package tracksview

import tea "charm.land/bubbletea/v2"

// The keys, clicks and hovers, routed to the tab they belong to, and
// switching between tabs.

// creatorFocused reports whether the theme creator has focus.
func (m Model) creatorFocused() bool {
	return m.tab == tabSettings && m.settings.section == sectionCreator && m.settings.editing
}

func (m Model) key(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch m.tab {
	case tabStation:
		if m.station.fast.Focused() && m.fastShown() {
			m.station.notice = notice{}
			return m.fastKey(msg)
		}
		m.station.fast.Blur()
	case tabProxy:
		m.proxy.notice = notice{}
		if m.proxy.adding {
			next, cmd, ok := m.proxyKey(msg)
			if ok {
				return next, cmd
			}
			m = next
		}
	case tabRepositories:
		m.repos.notice = notice{}
		if m.repos.editing {
			next, cmd := m.repoFormKey(msg)
			return next.scrollForm(), cmd
		}
	case tabEngines:
		m.engines.notice = notice{}
		if m.engines.editing {
			return m.enginesKey(msg)
		}
	case tabSettings:
		m.settings.notice = notice{}
		if m.settings.editing {
			return m.settingsKey(msg)
		}
	}
	switch msg.String() {
	case "tab":
		return m.switchTab((m.tab + 1) % len(tabs))
	case "shift+tab":
		return m.switchTab((m.tab + len(tabs) - 1) % len(tabs))
	}
	switch m.tab {
	case tabStation:
		if next, cmd, ok := m.stationKey(msg.String()); ok {
			return next, cmd
		}
	case tabProxy:
		if next, cmd, ok := m.proxyKey(msg); ok {
			return next, cmd
		}
	case tabRepositories:
		if next, cmd, ok := m.repoListKey(msg.String()); ok {
			return next, cmd
		}
	case tabEngines:
		if next, cmd, ok := m.enginesListKey(msg.String()); ok {
			return next, cmd
		}
	case tabSettings:
		if next, cmd, ok := m.settingsListKey(msg.String()); ok {
			return next, cmd
		}
	}
	return m, nil
}

func (m Model) click(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	if mouse.Button != tea.MouseLeft {
		return m, nil
	}
	if m.onNoticeClose(mouse.X, mouse.Y) {
		*m.noticeOf(m.tab) = notice{}
		return m, nil
	}
	if i, ok := m.tabAt(mouse.X, mouse.Y); ok {
		switch m.tab {
		case tabProxy:
			m = m.stopAdding()
		case tabRepositories:
			return m.request(leave{kind: leaveTab, tab: i})
		case tabSettings:
			return m.settingsRequest(settingsLeave{section: -1, tab: i})
		}
		return m.switchTab(i)
	}
	switch m.tab {
	case tabStation:
		return m.stationClick(mouse.X, mouse.Y)
	case tabProxy:
		return m.proxyClick(mouse.X, mouse.Y)
	case tabRepositories:
		next, cmd := m.repoClick(mouse.X, mouse.Y)
		return next.scrollForm(), cmd
	case tabEngines:
		return m.enginesClick(mouse.X, mouse.Y)
	case tabSettings:
		return m.settingsClick(mouse.X, mouse.Y)
	}
	return m, nil
}

func (m Model) hover(mouse tea.Mouse) Model {
	m.station.hover, m.repos.hover = -1, -1
	m.station.hoverButton, m.repos.hoverField, m.repos.hoverNew = -1, -1, false
	m.station.hoverAdd, m.station.hoverClear, m.station.hoverFast = false, false, false
	m.engines.hover = engineControl{}
	switch m.tab {
	case tabStation:
		if row, ok := m.rowAt(mouse.X, mouse.Y); ok {
			m.station.hover = row
		}
		if id, ok := m.buttonAt(mouse.X, mouse.Y); ok {
			m.station.hoverButton = id
		}
		m.station.hoverAdd = m.onAddTrack(mouse.X, mouse.Y)
		m.station.hoverClear = m.onClearFilter(mouse.X, mouse.Y)
		m.station.hoverFast = m.onFastButton(mouse.X, mouse.Y)
	case tabProxy:
		return m.proxyHover(mouse.X, mouse.Y)
	case tabRepositories:
		if row, ok := m.repoRowAt(mouse.X, mouse.Y); ok {
			m.repos.hover = row
		}
		m.repos.hoverNew = m.onNewRepo(mouse.X, mouse.Y)
		if field, ok := m.repoFieldAt(mouse.X, mouse.Y); ok {
			m.repos.hoverField = field
		}
	case tabEngines:
		return m.enginesHover(mouse.X, mouse.Y)
	case tabSettings:
		return m.settingsHover(mouse.X, mouse.Y)
	}
	return m
}

// switchTab shows tab i. Repositories reads the repos again, since the
// running tracks may have changed, Engines looks for the CLIs, and
// Settings reads the theme files.
func (m Model) switchTab(i int) (Model, tea.Cmd) {
	m = m.stopPrefix()
	m.tab = i
	switch i {
	case tabProxy:
		return m.loadProxy(true)
	case tabRepositories:
		return m, m.loadRepos()
	case tabEngines:
		m.engines.editing = false
		m.engines.input.Blur()
		return m.loadEngines()
	case tabSettings:
		return m, tea.Batch(m.loadThemes(), m.loadTypes(), m.loadAgents(), m.loadHistory(), m.loadNotify(), m.loadGitHub())
	}
	return m, nil
}
