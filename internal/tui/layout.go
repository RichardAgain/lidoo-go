package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"lidoo/internal/docker"
)

// frameGeometry describes the frame: a status row on top, the profile list and
// the detail pane, then the footer. Wide terminals put the two panes side by
// side around a separator; narrow ones stack the list above the detail pane.
// All y values are screen rows.
type frameGeometry struct {
	compact      bool
	leftWidth    int // profile list width
	rightWidth   int // detail pane width, including its one-column left pad
	detailLeft   int // x of the first detail column
	mainHeight   int // side-by-side only: rows between the status line and the footer rule
	listTop      int // y of the first profile row
	listRows     int // profile rows that fit
	detailTop    int // y of the profile header
	detailHeight int // rows available to the detail pane
}

const (
	frameChromeRows = 3 // status line, footer rule, footer
	compactMinWidth = 72
	compactMinRows  = 14
)

func (m *Model) compact() bool {
	return m.width < compactMinWidth || m.height < compactMinRows
}

func (m *Model) frame() frameGeometry {
	if m.compact() {
		// Status line and footer frame a short profile list, a rule, then
		// the detail pane.
		listRows := min(max(len(m.profiles), 1), max(1, (m.height-8)/3))
		detailTop := 1 + listRows + 1
		return frameGeometry{
			compact:      true,
			leftWidth:    m.width,
			rightWidth:   max(1, m.width-1),
			detailLeft:   1,
			listTop:      1,
			listRows:     listRows,
			detailTop:    detailTop,
			detailHeight: max(1, m.height-1-detailTop),
		}
	}
	leftWidth := min(max(m.width/4, 26), 34)
	mainHeight := max(1, m.height-frameChromeRows)
	return frameGeometry{
		leftWidth:    leftWidth,
		rightWidth:   max(1, m.width-leftWidth-2),
		detailLeft:   leftWidth + 2,
		mainHeight:   mainHeight,
		listTop:      3,
		listRows:     max(1, mainHeight-2),
		detailTop:    1,
		detailHeight: mainHeight,
	}
}

func (m *Model) view() string {
	var view string
	switch {
	case m.logViewerOpen:
		view = m.logViewerView()
	case m.compact():
		view = m.compactView()
	default:
		geometry := m.frame()
		lines := []string{m.statusLine()}
		lines = append(lines, m.mainLines(geometry)...)
		lines = append(lines, m.footerRule(geometry), m.footerLine(m.width))
		view = strings.Join(lines, "\n")
	}
	view = m.fitView(view)
	if m.modal != modalNone {
		return m.modalOverlay(view)
	}
	return view
}

// mainLines joins the profile list and the detail pane around the separator.
func (m *Model) mainLines(geometry frameGeometry) []string {
	if m.noProfiles() {
		return centeredLines("No profiles yet · n to create one", m.width, geometry.mainHeight)
	}
	left := fitLines(strings.Split(m.leftView(geometry), "\n"), geometry.leftWidth, geometry.mainHeight)
	right := fitLines(strings.Split(m.rightView(geometry), "\n"), geometry.rightWidth, geometry.mainHeight)
	separator := ruleStyle.Render("│")
	lines := make([]string, geometry.mainHeight)
	for index := range lines {
		lines[index] = left[index] + separator + " " + right[index]
	}
	return lines
}

// compactView stacks the profile list above the open tab for narrow or short
// terminals.
func (m *Model) compactView() string {
	geometry := m.frame()
	lines := []string{m.statusLine()}
	if m.noProfiles() {
		lines = append(lines, centeredLines("No profiles yet · n to create one", m.width, max(1, m.height-2))...)
		return strings.Join(append(lines, m.footerLine(m.width)), "\n")
	}
	lines = append(lines, fitLines(strings.Split(m.leftView(geometry), "\n"), m.width, geometry.listRows)...)
	lines = append(lines, ruleStyle.Render(strings.Repeat("─", m.width)))
	for _, line := range fitLines(strings.Split(m.rightView(geometry), "\n"), geometry.rightWidth, geometry.detailHeight) {
		lines = append(lines, " "+line)
	}
	return strings.Join(append(lines, m.footerLine(m.width)), "\n")
}

func (m *Model) noProfiles() bool {
	return len(m.profiles) == 0 && !m.loading && m.taskRegion(m.width, 3) == nil
}

func centeredLines(message string, width, height int) []string {
	lines := make([]string, height)
	row := height / 2
	for index := range lines {
		if index == row {
			lines[index] = lipgloss.PlaceHorizontal(width, lipgloss.Center, mutedStyle.Render(message))
		}
	}
	return lines
}

// fitLines truncates or pads each line to width and the slice to height.
func fitLines(lines []string, width, height int) []string {
	fitted := make([]string, height)
	for index := range fitted {
		line := ""
		if index < len(lines) {
			line = ansi.Truncate(lines[index], width, "")
		}
		fitted[index] = line + strings.Repeat(" ", max(0, width-ansi.StringWidth(line)))
	}
	return fitted
}

func (m *Model) footerRule(geometry frameGeometry) string {
	return ruleStyle.Render(strings.Repeat("─", geometry.leftWidth) + "┴" + strings.Repeat("─", max(0, m.width-geometry.leftWidth-1)))
}

// statusLine is the one-row header: app name, counts, a transient message and
// the Docker event watcher state.
func (m *Model) statusLine() string {
	running := 0
	for _, profile := range m.profiles {
		if profileRunning(profile.State) {
			running++
		}
	}
	left := " " + titleStyle.Render("lidoo") + "   " +
		secondStyle.Render(fmt.Sprintf("%d profiles · %d running", len(m.profiles), running))
	events := mutedStyle.Render("events ") + runningStyle.Render("●") + " " + secondStyle.Render("live")
	if m.watcherDisconnected {
		events = mutedStyle.Render("events ") + warningStyle.Render("disconnected")
	}
	events += " "
	message := ""
	switch {
	case m.err != nil:
		message = errorStyle.Render(m.err.Error())
	case m.notice != "":
		message = warningStyle.Render(m.notice)
	case m.loading:
		message = mutedStyle.Render("loading profiles…")
	}
	room := m.width - lipgloss.Width(left) - lipgloss.Width(events) - 3
	if message != "" && room > 0 {
		left += "   " + ansi.Truncate(message, room, "…")
	}
	gap := max(1, m.width-lipgloss.Width(left)-lipgloss.Width(events))
	return left + strings.Repeat(" ", gap) + events
}

func profileRunning(state string) bool {
	switch strings.ToLower(state) {
	case "running", "healthy":
		return true
	}
	return false
}

// profilesFocused reports whether the profile list owns the keyboard.
func (m *Model) profilesFocused() bool {
	return m.pane == paneProfiles
}

// profileListBudget is how many profile rows fit in the list.
func (m *Model) profileListBudget() int {
	return m.frame().listRows
}

// profileWindowStart is the first visible profile; the window always contains
// the selection.
func (m *Model) profileWindowStart() int {
	return windowStart(len(m.profiles), m.profileIndex, m.profileListBudget())
}

func (m *Model) leftView(geometry frameGeometry) string {
	budget := geometry.listRows
	var lines []string
	if !geometry.compact {
		title := " " + mutedStyle.Bold(true).Render("PROFILES")
		if len(m.profiles) > budget {
			title += " " + dimStyle.Render(fmt.Sprintf("%d/%d", m.profileIndex+1, len(m.profiles)))
		}
		lines = []string{title, ""}
	}
	if len(m.profiles) == 0 {
		if m.loading {
			lines = append(lines, " "+mutedStyle.Render("loading..."))
		} else {
			lines = append(lines, " "+mutedStyle.Render("no profiles found"))
		}
		return strings.Join(lines, "\n")
	}
	versionWidth := 2
	for _, profile := range m.profiles {
		versionWidth = max(versionWidth, min(5, lipgloss.Width(profile.Version)))
	}
	start := m.profileWindowStart()
	end := min(len(m.profiles), start+budget)
	for index := start; index < end; index++ {
		lines = append(lines, m.profileRow(geometry.leftWidth, m.profiles[index], index == m.profileIndex, versionWidth))
	}
	return strings.Join(lines, "\n")
}

// profileRow renders one profile on one line: marker, status glyph, name and
// Odoo version. The selected row of the focused pane gets the rule background.
func (m *Model) profileRow(width int, profile docker.ProfileSummary, selected bool, versionWidth int) string {
	focused := selected && m.profilesFocused()
	segment := func(style lipgloss.Style, value string) string {
		if focused {
			style = style.Background(colorRule)
		}
		return style.Render(value)
	}

	marker := segment(lipgloss.NewStyle(), " ")
	nameStyle := secondStyle
	switch {
	case focused:
		marker = segment(activeStyle, "▌")
		nameStyle = activeStyle
	case selected:
		marker = dimStyle.Render("▌")
		nameStyle = textStyle
	}

	glyphStyle, glyph := dimStyle, "○"
	switch {
	case m.taskActive() && m.taskProfileName == profile.Name:
		glyphStyle, glyph = activeStyle, m.spinnerFrame()
	case profileRunning(profile.State):
		glyphStyle, glyph = runningStyle, "●"
	}

	nameWidth := max(1, width-5-1-versionWidth-1)
	pending := profile.PendingRecreation.Pending
	name := profile.Name
	if pending {
		name = ansi.Truncate(name, max(1, nameWidth-2), "…")
	} else {
		name = ansi.Truncate(name, nameWidth, "…")
	}
	used := lipgloss.Width(name)
	row := segment(lipgloss.NewStyle(), " ") + marker + segment(lipgloss.NewStyle(), " ") + segment(glyphStyle, glyph) +
		segment(lipgloss.NewStyle(), " ") + segment(nameStyle, name)
	if pending {
		row += segment(lipgloss.NewStyle(), " ") + segment(warningStyle, "↻")
		used += 2
	}
	row += segment(lipgloss.NewStyle(), strings.Repeat(" ", max(0, nameWidth-used)+1))
	version := ansi.Truncate(profile.Version, versionWidth, "")
	row += segment(secondStyle, strings.Repeat(" ", versionWidth-lipgloss.Width(version))+version)
	row += segment(lipgloss.NewStyle(), " ")
	return row
}

// spinnerFrame is the current spinner glyph without its style, so callers can
// colour it to match the surrounding row.
func (m *Model) spinnerFrame() string {
	return ansi.Strip(m.activitySpinner.View())
}

// profileHeaderLines is the number of rows profileHeader produces, including
// the blank row that separates it from the tab bar.
func (m *Model) profileHeaderLines() int {
	if m.selectedProfile() != nil && m.selectedProfile().PendingRecreation.Pending {
		return 4
	}
	return 3
}

// profileHeader names the selected profile, its state and its URL.
func (m *Model) profileHeader() []string {
	profile := m.selectedProfile()
	if profile == nil {
		return []string{"", "", ""}
	}
	state := mutedStyle.Render("○ " + profileStateText(profile.State))
	switch {
	case m.taskActive() && m.taskProfileName == profile.Name:
		state = activeStyle.Render(m.spinnerFrame() + " " + taskActionLabel(m.activeTaskKind))
	case profileRunning(profile.State):
		state = runningStyle.Render("●") + " " + secondStyle.Render(profileStateText(profile.State))
	}
	line := titleStyle.Render(profile.Name) + "  " + state
	if profile.Version != "" {
		line += "  " + mutedStyle.Render("·") + "  " + secondStyle.Render("Odoo "+profile.Version)
	}
	lines := []string{line, mutedStyle.Render(profile.URL)}
	if profile.PendingRecreation.Pending {
		note := "↻ pending recreation"
		if reason := profile.PendingRecreation.Reason; reason != "" {
			note += " · " + reason
		}
		lines = append(lines, warningStyle.Render(note))
	}
	return append(lines, "")
}

func profileStateText(state string) string {
	if state == "" {
		return "not created"
	}
	return state
}

type detailTab struct {
	name string
	key  string
}

var detailTabs = []detailTab{
	{name: "Logs", key: "1"},
	{name: "Databases", key: "2"},
	{name: "Add-ons", key: "3"},
	{name: "Info", key: "4"},
	{name: "Config", key: "5"},
}

// activeTab maps the focus area to the tab shown in the detail pane.
func (m *Model) activeTab() int {
	return int(m.tab)
}

// tabGap is the space between tab labels; it tightens on narrow terminals so
// the whole bar stays visible.
func (m *Model) tabGap() int {
	if m.frame().rightWidth >= 58 {
		return 3
	}
	return 2
}

// tabBar renders the numbered tabs and the underline below the active one.
func (m *Model) tabBar() (string, string) {
	active := m.activeTab()
	var labels, underline strings.Builder
	labels.WriteString(" ")
	underline.WriteString(" ")
	gap := strings.Repeat(" ", m.tabGap())
	for index, tab := range detailTabs {
		if index > 0 {
			labels.WriteString(gap)
			underline.WriteString(gap)
		}
		text := tab.key + " " + tab.name
		if index == active {
			style := secondStyle.Bold(true)
			if !m.profilesFocused() {
				style = activeStyle
			}
			labels.WriteString(style.Render(text))
			underline.WriteString(style.Render(strings.Repeat("━", lipgloss.Width(text))))
			continue
		}
		labels.WriteString(keyStyle.Foreground(colorMuted).Render(tab.key) + " " + mutedStyle.Render(tab.name))
		underline.WriteString(strings.Repeat(" ", lipgloss.Width(text)))
	}
	return labels.String(), underline.String()
}

// detailLayout splits the detail pane into the profile header, the tab
// content and the task region.
type detailLayout struct {
	header        []string
	task          []string
	contentWidth  int
	contentHeight int
}

func (m *Model) detailLayout(geometry frameGeometry) detailLayout {
	layout := detailLayout{header: m.profileHeader()}
	layout.task = m.taskRegion(geometry.rightWidth-1, geometry.detailHeight/2)
	for index := range layout.task {
		layout.task[index] = " " + layout.task[index]
	}
	layout.contentWidth = max(1, geometry.rightWidth-1)
	layout.contentHeight = max(1, geometry.detailHeight-len(layout.header)-2-len(layout.task))
	return layout
}

func (m *Model) rightView(geometry frameGeometry) string {
	layout := m.detailLayout(geometry)
	labels, underline := m.tabBar()
	width, height := layout.contentWidth, layout.contentHeight

	var content string
	switch m.tab {
	case tabLogs:
		content = m.logView(width, height)
	case tabDatabases:
		content = m.databaseTabView(width, height)
	case tabAddons:
		content = m.addonTabView(width, height)
	case tabConfig:
		content = m.configTabView(width)
	default:
		content = m.infoTabView()
	}
	content = lipgloss.NewStyle().Width(width + 1).MaxWidth(width + 1).PaddingLeft(1).Height(height).MaxHeight(height).Render(content)
	lines := append(layout.header, labels, underline, content)
	return strings.Join(append(lines, layout.task...), "\n")
}

const taskOutputLines = 6

// taskRegion is the bottom block of the detail pane: a rule, the running task
// with its elapsed time, and the tail of its output. A finished task collapses
// to one line until it is expanded with t.
func (m *Model) taskRegion(width, maxLines int) []string {
	if m.taskStatus == "" || m.taskName == "" {
		return nil
	}
	active := m.taskActive() || m.interactivePreparing
	if !active && !m.taskExpanded {
		suffix := "  " + taskStatus(m.taskStatus) + mutedStyle.Render("  · t expand")
		name := ansi.Truncate(m.taskName, max(0, width-lipgloss.Width(suffix)), "…")
		return []string{name + suffix}
	}
	lines := []string{ruleStyle.Render("── ") + mutedStyle.Render("task") + " " + ruleStyle.Render(strings.Repeat("─", max(0, width-8)))}

	var title, hint string
	if active {
		title = activeStyle.Render(m.spinnerFrame()) + " " + textStyle.Render(m.taskName)
		if !m.taskStartedAt.IsZero() {
			title += "   " + mutedStyle.Render(formatElapsed(time.Since(m.taskStartedAt)))
		}
		hint = keyStyle.Render("c") + " " + mutedStyle.Render("cancel")
	} else {
		title = textStyle.Render(m.taskName) + "  " + taskStatus(m.taskStatus)
		hint = mutedStyle.Render("t collapse")
	}
	gap := width - lipgloss.Width(title) - lipgloss.Width(hint)
	if gap < 1 {
		title = ansi.Truncate(title, max(1, width-lipgloss.Width(hint)-1), "…")
		gap = max(1, width-lipgloss.Width(title)-lipgloss.Width(hint))
	}
	lines = append(lines, title+strings.Repeat(" ", gap)+hint)
	if m.taskNotice != "" {
		lines = append(lines, errorStyle.Render("  "+m.taskNotice))
	}
	if m.taskErr != nil {
		lines = append(lines, errorStyle.Render("  "+m.taskErr.Error()))
	}
	if tail, truncated := taskOutputTail(m.taskOutput, taskOutputLines); tail != "" {
		if truncated {
			lines = append(lines, mutedStyle.Render("  … older output above"))
		}
		lines = append(lines, strings.Split(tail, "\n")...)
	}
	for index, line := range lines {
		lines[index] = ansi.Truncate(line, width, "…")
	}
	if maxLines >= 3 && len(lines) > maxLines {
		lines = append(lines[:2], lines[len(lines)-maxLines+2:]...)
	}
	return lines
}

func formatElapsed(elapsed time.Duration) string {
	seconds := int(elapsed.Seconds())
	if seconds >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", seconds/3600, seconds/60%60, seconds%60)
	}
	return fmt.Sprintf("%d:%02d", seconds/60, seconds%60)
}
