package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"lidoo/internal/docker"
)

// frameGeometry describes the two-pane frame: a status row on top, the
// profile list and the detail pane side by side, then a rule and the footer.
type frameGeometry struct {
	leftWidth  int // profile pane width; the separator sits in the next column
	rightWidth int // detail pane content width (after the separator and a space)
	mainHeight int // rows between the status line and the footer rule
}

const frameChromeRows = 3 // status line, footer rule, footer

func (m *Model) frame() frameGeometry {
	leftWidth := min(max(m.width/4, 26), 34)
	return frameGeometry{
		leftWidth:  leftWidth,
		rightWidth: max(1, m.width-leftWidth-2),
		mainHeight: max(1, m.height-frameChromeRows),
	}
}

func (m *Model) view() string {
	if m.logViewerOpen {
		view := m.logViewerView()
		if m.modal != modalNone {
			return m.modalOverlay(m.fitView(view))
		}
		return m.fitView(view)
	}
	if m.width < 72 || m.height < 14 {
		view := m.smallView()
		if m.modal != modalNone {
			return m.modalOverlay(m.fitView(view))
		}
		return m.fitView(view)
	}

	geometry := m.frame()
	lines := []string{m.statusLine()}
	lines = append(lines, m.mainLines(geometry)...)
	lines = append(lines, m.footerRule(geometry), m.footerView())
	view := m.fitView(strings.Join(lines, "\n"))
	if m.modal != modalNone {
		return m.modalOverlay(view)
	}
	return view
}

// mainLines joins the profile list and the detail pane around the separator.
func (m *Model) mainLines(geometry frameGeometry) []string {
	if len(m.profiles) == 0 && !m.loading && m.taskRegion(m.width, 3) == nil {
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
	return m.focus == focusProfiles
}

// profileListBudget is how many profile rows fit under the pane title.
func (m *Model) profileListBudget() int {
	return max(1, m.frame().mainHeight-2)
}

// profileWindowStart is the first visible profile; the window always contains
// the selection.
func (m *Model) profileWindowStart() int {
	return windowStart(len(m.profiles), m.profileIndex, m.profileListBudget())
}

func (m *Model) leftView(geometry frameGeometry) string {
	title := " " + mutedStyle.Bold(true).Render("PROFILES")
	budget := m.profileListBudget()
	if len(m.profiles) > budget {
		title += " " + dimStyle.Render(fmt.Sprintf("%d/%d", m.profileIndex+1, len(m.profiles)))
	}
	lines := []string{title, ""}
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
	return int(m.focus)
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

func (m *Model) rightView(geometry frameGeometry) string {
	width := geometry.rightWidth
	header := m.profileHeader()
	labels, underline := m.tabBar()
	task := m.taskRegion(width-1, geometry.mainHeight/2)
	for index := range task {
		task[index] = " " + task[index]
	}
	contentHeight := max(1, geometry.mainHeight-len(header)-2-len(task))

	width--
	var content string
	switch m.focus {
	case focusProfiles:
		content = m.logView(width, contentHeight)
	case focusDatabases:
		content = m.databaseTabView(width)
	case focusAddons:
		content = m.addonTabView(width)
	case focusConfig:
		content = m.configTabView()
	default:
		content = m.profileDetailView()
	}
	if m.showHelp {
		content = m.helpView()
	}
	content = lipgloss.NewStyle().Width(width + 1).MaxWidth(width + 1).PaddingLeft(1).Height(contentHeight).MaxHeight(contentHeight).Render(content)
	width++
	lines := append(header, labels, underline, content)
	return strings.Join(append(lines, task...), "\n")
}

func (m *Model) helpView() string {
	return strings.Join([]string{
		titleStyle.Render("Help"),
		"New profiles are saved without starting a container; space starts them.",
		"Settings are saved to .lidoo.json; R recreates the container to apply them.",
		"Database filters match physical names, e.g. ^smoke__(dev|staging)$.",
		"admin_passwd protects /web/database; it is not the admin login password.",
		"The admin password action changes the base.user_admin login via Odoo.",
		"LAN port: 0 disables; 1–65535 publishes on all IPv4 interfaces.",
		"Share http://<host-LAN-IP>:<port>; restrict access with your firewall.",
		"Restore copy requires a new destination; move replaces the destination.",
		"Missing worktree branches are created; parent/worktree rules still apply.",
	}, "\n")
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
