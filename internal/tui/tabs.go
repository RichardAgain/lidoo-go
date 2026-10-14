package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"lidoo/internal/addons"
)

const (
	databaseNameMaxWidth = 36
	addonNameMaxWidth    = 30
)

// logView is the Logs tab: a one-line filter strip above the log viewport.
func (m *Model) logView(width, height int) string {
	return m.logStrip(width) + "\n" + m.logContainer(width, height-1, m.logContent())
}

func (m *Model) logStrip(width int) string {
	database := "all"
	if m.logDatabase != "" {
		database = m.logDatabase
	}
	follow := runningStyle.Render("●")
	if !m.logViewport.AtBottom() {
		follow = dimStyle.Render("○")
	}
	left := mutedStyle.Render("level all · db "+database+" · follow ") + follow
	right := keyStyle.Render("/") + " " + mutedStyle.Render("search") + "  " + keyStyle.Render("L") + " " + mutedStyle.Render("fullscreen")
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		// Narrow pane: the level is always "all" here, so drop it first.
		left = mutedStyle.Render("db "+database+" · follow ") + follow
		gap = width - lipgloss.Width(left) - lipgloss.Width(right)
	}
	if gap < 1 {
		return ansi.Truncate(left, width, "…")
	}
	return left + strings.Repeat(" ", gap) + right
}

func (m *Model) databaseTabView(width, height int) string {
	switch m.databasePhase {
	case phaseLoading:
		return mutedStyle.Render("loading databases...")
	case phaseEmpty:
		return strings.Join([]string{
			mutedStyle.Render("no databases found"),
			"",
			keyStyle.Render("i") + " " + mutedStyle.Render("create an empty database"),
			keyStyle.Render("R") + " " + mutedStyle.Render("restore a database from a dump file"),
		}, "\n")
	case phaseUnavailable:
		if len(m.databases) == 0 {
			return warningStyle.Render("databases unavailable while stopped")
		}
		return m.databaseTable(width, height, warningStyle.Render("unavailable while stopped; showing last snapshot"))
	case phaseError:
		if len(m.databases) == 0 {
			return errorStyle.Render("refresh failed: " + errorText(m.databaseErr))
		}
		return m.databaseTable(width, height, errorStyle.Render("refresh failed; showing last snapshot"))
	case phaseReady:
		view := m.databaseTable(width, height-boolInt(m.databaseErr != nil), "")
		if m.databaseErr != nil {
			view += "\n" + errorStyle.Render("refresh failed; showing last snapshot: "+errorText(m.databaseErr))
		}
		return view
	}
	return mutedStyle.Render("not loaded")
}

// databaseTable renders the database list under an optional warning line.
func (m *Model) databaseTable(width, height int, warning string) string {
	rows := make([][]tableCell, 0, len(m.databases))
	for _, database := range m.databases {
		name := database.Logical
		if name == "" {
			name = database.Physical
		}
		size := tableCell{Text: "—", Style: secondStyle}
		state := tableCell{Text: "unknown", Style: mutedStyle}
		if info, ok := m.databaseInfos[database.Physical]; ok {
			if info.Size != "" {
				size.Text = info.Size
			}
			state = databaseState(info.Available)
		} else if m.databaseInfoPhase == phaseLoading {
			size.Text = "loading…"
			state = tableCell{Text: "checking…", Style: mutedStyle}
		}
		if m.databasePhase == phaseUnavailable {
			state = databaseState(false)
		}
		rows = append(rows, []tableCell{{Text: name, Style: textStyle}, size, state})
	}
	top := 0
	if warning != "" {
		top = 2
		height -= top
	}
	view, start := (dataTable{
		Columns: []dataTableColumn{
			{Title: "NAME", Flex: 1, MinWidth: 12, MaxWidth: databaseNameMaxWidth},
			{Title: "SIZE", Width: 8, Right: true},
			{Title: "STATE", Width: 13},
		},
		Rows:        rows,
		SelectedRow: m.databaseIndex,
		Width:       width,
		Height:      height,
		Focused:     m.pane == paneDetails,
	}).View()
	m.databaseTableTop, m.databaseTableStart = top, start
	if warning != "" {
		return warning + "\n\n" + view
	}
	return view
}

func databaseState(available bool) tableCell {
	if available {
		return tableCell{Text: "ready", Style: secondStyle, Glyph: "●", GlyphStyle: runningStyle}
	}
	return tableCell{Text: "unavailable", Style: secondStyle, Glyph: "○", GlyphStyle: dimStyle}
}

func (m *Model) addonTabView(width, height int) string {
	switch m.addonPhase {
	case phaseLoading:
		return mutedStyle.Render("loading...")
	case phaseEmpty:
		message := mutedStyle.Render("no add-ons registered") + "\n\n" +
			keyStyle.Render("c") + " " + mutedStyle.Render("clone an add-on repository") + "\n" +
			keyStyle.Render("w") + " " + mutedStyle.Render("create a worktree from a registered add-on")
		if m.addonErr != nil {
			return message + "\n" + errorStyle.Render("refresh failed: "+errorText(m.addonErr))
		}
		return message
	case phaseError:
		return errorStyle.Render("refresh failed")
	case phaseReady:
		return m.addonTable(width, height)
	}
	return mutedStyle.Render("not loaded")
}

func (m *Model) addonTable(width, height int) string {
	profileName := m.selectedProfileName()
	rows := make([][]tableCell, 0, len(m.addons))
	for _, addon := range m.addons {
		rows = append(rows, m.addonRow(addon, profileName))
	}
	if m.addonErr != nil {
		height--
	}
	view, start := (dataTable{
		Columns: []dataTableColumn{
			{Title: "", Width: 1, Tight: true},
			{Title: "NAME", Flex: 2, MinWidth: 12, MaxWidth: addonNameMaxWidth},
			{Title: "BRANCH", Flex: 1, MinWidth: 8},
			{Title: "SOURCE", Flex: 2, MinWidth: 10},
		},
		Rows:        rows,
		SelectedRow: m.addonIndex,
		Width:       width,
		Height:      height,
		Focused:     m.pane == paneDetails,
	}).View()
	m.addonTableStart = start
	if m.addonErr != nil {
		view += "\n" + errorStyle.Render("refresh failed: "+errorText(m.addonErr))
	}
	return view
}

func (m *Model) addonRow(addon addons.AddonStatus, profileName string) []tableCell {
	branch := addon.Branch
	if branch == "" {
		branch = "—"
	}
	source := addon.Entry.Source
	if addon.Kind == "worktree" {
		source = "↳ " + addon.Entry.WorktreeOf
	}
	if addon.Entry.Source == "" && addon.Kind != "worktree" {
		source = "—"
	}
	check := tableCell{Text: " "}
	if addonAttached(addon, profileName) {
		check = tableCell{Text: "✓", Style: runningStyle}
	}
	return []tableCell{
		check,
		{Text: addon.Name, Style: textStyle},
		{Text: branch, Style: secondStyle},
		{Text: source, Style: mutedStyle},
	}
}

// infoRow aligns a muted key with its value.
func infoRow(key, value string) string {
	return mutedStyle.Render(fmt.Sprintf("%-12s", key)) + value
}

func (m *Model) infoTabView() string {
	if m.selectedProfileName() == "" {
		return mutedStyle.Render("No profile selected")
	}
	switch m.profilePhase {
	case phaseLoading:
		return mutedStyle.Render("Loading profile details...")
	case phaseError:
		return errorStyle.Render(errorText(m.profileDetailErr))
	}
	detail := m.profileDetail
	state := mutedStyle.Render("○ " + profileStateText(detail.DockerState))
	if profileRunning(detail.DockerState) {
		state = runningStyle.Render("●") + " " + textStyle.Render(detail.DockerState)
	}
	odooVersion := textStyle.Render(detail.OdooVersion)
	if detail.Image != "" {
		odooVersion += mutedStyle.Render(" · " + detail.Image)
	}
	addonNames := mutedStyle.Render("none")
	if len(detail.AttachedAddons) > 0 {
		addonNames = textStyle.Render(strings.Join(detail.AttachedAddons, ", "))
	}
	recreation := secondStyle.Render(detail.PendingRecreation.String())
	if detail.PendingRecreation.Pending {
		recreation = warningStyle.Render("↻ " + detail.PendingRecreation.String())
	}
	lines := []string{
		infoRow("State", state),
		infoRow("URL", textStyle.Render(detail.URL)),
		infoRow("Odoo", odooVersion),
		infoRow("Add-ons", addonNames),
		infoRow("DB prefix", textStyle.Render(detail.DatabasePrefix)),
		infoRow("Filestore", textStyle.Render(detail.FilestoreVolume)),
		infoRow("LAN port", textStyle.Render(lanPortLabel(detail.LANPort))),
		infoRow("Recreation", recreation),
	}
	if m.profileDetailErr != nil {
		lines = append(lines, "", errorStyle.Render("refresh failed; showing last snapshot: "+errorText(m.profileDetailErr)))
	}
	return strings.Join(lines, "\n")
}

func lanPortLabel(port int) string {
	if port == 0 {
		return "disabled"
	}
	return strconv.Itoa(port)
}

// configTabView lists the profile settings as rows; enter edits the selected
// one.
func (m *Model) configTabView(width int) string {
	if m.selectedProfileName() == "" {
		return mutedStyle.Render("No profile selected")
	}
	switch {
	case m.configPhase == phaseLoading || (m.configPhase == phaseIdle && m.profilePhase == phaseLoading):
		return mutedStyle.Render("loading configuration...")
	case m.configPhase == phaseError:
		return errorStyle.Render("refresh failed: " + errorText(m.configErr))
	case m.configPhase != phaseReady:
		return mutedStyle.Render("configuration unavailable")
	}
	rows := m.profileSettingRows()
	lines := make([]string, 0, len(rows)+4)
	for index, row := range rows {
		label, value, style := m.profileSettingValue(row)
		lines = append(lines, m.configRow(width, label, value, style, index == m.profileSettingsIndex))
	}
	if m.configFormErr != nil {
		lines = append(lines, "", errorStyle.Render(m.configFormErr.Error()))
	}
	if pending := m.profileDetail.PendingRecreation; pending.Pending {
		note := "↻ changes waiting"
		if pending.Reason != "" {
			note += " (" + pending.Reason + ")"
		}
		lines = append(lines, "", warningStyle.Render(note+" · ")+keyStyle.Render("R")+" "+mutedStyle.Render("recreates the container to apply them"))
	}
	if port := m.profileDetail.LANPort; port != 0 && m.currentSettingRow() == settingLANPort {
		lines = append(lines, "", mutedStyle.Render(fmt.Sprintf("Share http://<host-LAN-IP>:%d · all IPv4 interfaces; restrict access with your firewall", port)))
	}
	if m.profileDetailErr != nil {
		lines = append(lines, errorStyle.Render("refresh failed: "+errorText(m.profileDetailErr)))
	}
	return strings.Join(lines, "\n")
}

func (m *Model) configRow(width int, label, value string, valueStyle lipgloss.Style, selected bool) string {
	focused := selected && m.pane == paneDetails
	segment := func(style lipgloss.Style, text string) string {
		if focused {
			style = style.Background(colorRule)
		}
		return style.Render(text)
	}
	marker := segment(lipgloss.NewStyle(), " ")
	labelStyle := mutedStyle
	switch {
	case focused:
		marker, labelStyle = segment(activeStyle, "▌"), activeStyle
	case selected:
		marker = dimStyle.Render("▌")
	}
	line := marker + segment(lipgloss.NewStyle(), " ") + segment(labelStyle, fmt.Sprintf("%-17s", label)) + segment(valueStyle, value)
	if gap := width - lipgloss.Width(line); gap > 0 {
		line += segment(lipgloss.NewStyle(), strings.Repeat(" ", gap))
	}
	return line
}

func (m *Model) currentSettingRow() profileSettingRow {
	rows := m.profileSettingRows()
	if m.profileSettingsIndex < 0 || m.profileSettingsIndex >= len(rows) {
		return settingDBFilterMode
	}
	return rows[m.profileSettingsIndex]
}
