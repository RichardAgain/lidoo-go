package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// focusPane is the pane that owns the keyboard: the profile list or the
// detail pane showing one tab.
type focusPane uint8

const (
	paneProfiles focusPane = iota
	paneDetails
)

type tabID uint8

const (
	tabLogs tabID = iota
	tabDatabases
	tabAddons
	tabInfo
	tabConfig
	tabCount
)

func (m *Model) onProfiles() bool {
	return m.pane == paneProfiles
}

// onTab reports whether the detail pane is focused on the given tab. Tab
// actions only apply then; the profile pane keeps its own actions.
func (m *Model) onTab(tab tabID) bool {
	return m.pane == paneDetails && m.tab == tab
}

func (m *Model) focusProfileList() {
	m.pane = paneProfiles
}

// focusDetails moves the keyboard to the open tab. It needs a profile to show.
func (m *Model) focusDetails() bool {
	if m.selectedProfileName() == "" {
		return false
	}
	m.pane = paneDetails
	return true
}

// setTab opens a tab without moving focus. Container logs only stream while
// the Logs tab is open.
func (m *Model) setTab(tab tabID) tea.Cmd {
	if tab >= tabCount || tab == m.tab {
		return nil
	}
	previous := m.tab
	m.tab = tab
	if tab == tabLogs {
		return m.beginLogLoad()
	}
	if previous == tabLogs {
		m.stopLogStream()
		m.logRequest++
	}
	return nil
}

// openTab switches to a tab and focuses the detail pane.
func (m *Model) openTab(tab tabID) tea.Cmd {
	if !m.focusDetails() {
		return nil
	}
	return m.setTab(tab)
}

// stepTab opens the previous or next tab, wrapping around.
func (m *Model) stepTab(delta int) tea.Cmd {
	return m.setTab(tabID((int(m.tab) + delta + int(tabCount)) % int(tabCount)))
}

// moveProfile selects another profile; the open tab and the focused pane stay.
func (m *Model) moveProfile(delta int) tea.Cmd {
	if !m.moveIndex(&m.profileIndex, len(m.profiles), delta) {
		return nil
	}
	m.profileListOnlyRefresh = false
	return m.beginProfileResources()
}

// navigationKey handles the keys that move between panes, tabs, profiles and
// rows. The second result reports whether the key was consumed.
func (m *Model) navigationKey(key string) (tea.Cmd, bool) {
	switch key {
	case "1", "2", "3", "4", "5":
		return m.openTab(tabID(key[0] - '1')), true
	case "o":
		if profile := m.selectedProfile(); profile != nil {
			return OpenProfileURLCmd(profile.URL), true
		}
		return nil, true
	}
	if m.onProfiles() {
		switch key {
		case "enter", "tab", "shift+tab", "right", "l":
			m.focusDetails()
			return nil, true
		case "up", "k":
			return m.moveProfile(-1), true
		case "down", "j":
			return m.moveProfile(1), true
		}
		return nil, false
	}
	switch key {
	case "esc", "tab", "shift+tab", "left", "h":
		m.focusProfileList()
		return nil, true
	case "[":
		return m.stepTab(-1), true
	case "]":
		return m.stepTab(1), true
	case "shift+up", "K":
		return m.moveProfile(-1), true
	case "shift+down", "J":
		return m.moveProfile(1), true
	case "up", "k":
		m.moveRow(-1)
		return nil, true
	case "down", "j":
		m.moveRow(1)
		return nil, true
	case "right", "l":
		return nil, true
	}
	return nil, false
}

// moveRow moves the row cursor of the open tab, or scrolls the Logs tab.
func (m *Model) moveRow(delta int) {
	switch m.tab {
	case tabLogs:
		if delta < 0 {
			m.logViewport.LineUp(1)
		} else {
			m.logViewport.LineDown(1)
		}
	case tabDatabases:
		m.moveIndex(&m.databaseIndex, len(m.databases), delta)
	case tabAddons:
		m.moveIndex(&m.addonIndex, len(m.addons), delta)
	case tabConfig:
		m.moveProfileSetting(delta)
	}
}
