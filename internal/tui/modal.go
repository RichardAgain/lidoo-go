package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	modalMinWidth = 44
	modalMaxWidth = 72
)

// colorBackdrop is the single colour every cell of the dashboard takes while a
// modal is open.
const colorBackdrop = lipgloss.Color("#3f3f46")

// modalSpec is a modal's content: a title (context after " · "), body lines and
// the key hints that go in the bottom border.
type modalSpec struct {
	title string
	body  []string
	keys  string
}

func (m *Model) modalSpec() modalSpec {
	switch m.modal {
	case modalConfirm:
		spec := modalSpec{title: m.confirmTitle, body: []string{warningStyle.Render(m.confirmPrompt)}, keys: "y/enter confirm  n/esc cancel"}
		if m.confirmForceToggle {
			force := "Force: " + yesNo(m.removeAddonForce)
			if m.removeAddonForce {
				force += " — may discard uncommitted changes"
			}
			spec.body = append(spec.body, force)
			spec.keys = "f toggle force  y/enter confirm  n/esc cancel"
		}
		return spec
	case modalRunVersion:
		return modalSpec{
			title: "Odoo version · " + m.taskProfileName,
			body:  []string{"version: " + activeStyle.Render(m.versionInput+"▏")},
			keys:  "type version  enter run  esc cancel",
		}
	case modalInitDatabase:
		return modalSpec{
			title: "Initialize database · " + m.taskProfileName,
			body: []string{
				m.formLine("database", m.initDatabaseName, 0, true),
				m.formLine("modules", m.initModules, 1, true),
			},
			keys: "tab field  enter run  esc cancel",
		}
	case modalBackupDatabase:
		destination := m.backupDestination
		if destination == "" {
			destination = "(default backups/<database>_...zip)"
		}
		return modalSpec{
			title: "Backup · " + m.taskProfileName + " / " + m.taskDatabaseName,
			body: []string{
				m.formLine("destination", destination, 0, m.formField == 0),
				m.formLine("format", m.backupFormat, 1, false),
				m.formLine("filestore", yesNo(m.backupFilestore), 2, false),
				m.formLine("overwrite", yesNo(m.backupForce), 3, false),
			},
			keys: "tab field  ctrl+o folder  space toggle  enter run  esc cancel",
		}
	case modalRestoreDatabase:
		source := m.restoreSource
		if source == "" {
			source = "(required)"
		}
		destination := m.restoreDestination
		if destination == "" {
			destination = "(required)"
		}
		body := []string{
			m.formLine("destination", destination, 0, true),
			m.formLine("source", source, 1, true),
			m.formLine("mode", restoreMode(m.restoreCopy), 2, false),
			m.formLine("overwrite", yesNo(m.restoreForce), 3, false),
			m.formLine("neutralize", yesNo(m.restoreNeutralize), 4, false),
			m.formLine("jobs", m.restoreJobs, 5, true),
		}
		if m.restoreErr != nil {
			body = append(body, errorStyle.Render(m.restoreErr.Error()))
		} else {
			body = append(body, mutedStyle.Render("copy: new database · move: replace destination"))
		}
		return modalSpec{title: "Restore database · " + m.taskProfileName, body: body, keys: "tab field  space toggle  enter run  esc cancel"}
	case modalCloneAddon:
		body := []string{
			m.formLine("name", m.cloneAddonName, 0, true),
			m.formLine("Git URL", m.cloneAddonURL, 1, true),
			m.formLine("depth (0 = full)", m.cloneAddonDepth, 2, true),
			m.formLine("branch (optional)", m.cloneAddonBranch, 3, true),
		}
		if m.addonFormErr != nil {
			body = append(body, errorStyle.Render(m.addonFormErr.Error()))
		}
		return modalSpec{title: "Clone and register add-on", body: body, keys: "tab field  enter clone  esc cancel"}
	case modalWorktreeAddon:
		body := []string{
			m.formLine("worktree name", m.worktreeAddonName, 0, true),
			m.formLine("branch", m.worktreeAddonBranch, 1, true),
		}
		if m.addonFormErr != nil {
			body = append(body, errorStyle.Render(m.addonFormErr.Error()))
		} else {
			body = append(body, mutedStyle.Render("Missing branches are created."))
		}
		return modalSpec{title: "Create worktree · " + m.worktreeAddonSource, body: body, keys: "tab field  enter create  esc cancel"}
	case modalKeys:
		return m.keysModalSpec()
	case modalSelect:
		return m.selectModalSpec()
	case modalFilePicker:
		return m.filePickerModalSpec()
	case modalCreateProfile:
		body := []string{
			m.formLine("profile name", m.createProfileName, 0, true),
			m.formLine("Odoo version", m.createProfileVersion, 1, true),
		}
		if m.createFormErr != nil {
			body = append(body, errorStyle.Render(m.createFormErr.Error()))
		} else {
			body = append(body, mutedStyle.Render("space starts the profile after creation"))
		}
		return modalSpec{title: "New profile", body: body, keys: "tab field  ←/→ version  type custom  enter create  esc cancel"}
	case modalConfigPattern:
		body := []string{m.formLine("regular expression", m.configPatternDraft, 0, true)}
		body = append(body, m.configHint("Matches physical database names."))
		return modalSpec{title: "Database filter pattern · " + m.configProfileName, body: body, keys: "enter apply  esc back"}
	case modalConfigPassword:
		body := []string{m.formLine("master password", maskSecret(m.configPasswordDraft), 0, true)}
		body = append(body, m.configHint("admin_passwd for /web/database; not the login password"))
		return modalSpec{title: "Odoo master password · " + m.configProfileName, body: body, keys: "enter apply  esc back"}
	case modalConfigLANPort:
		body := []string{m.formLine("host port", m.configLANPortDraft, 0, true)}
		body = append(body, m.configHint("0 disables LAN access; R recreates to apply"))
		return modalSpec{title: "LAN port · " + m.configProfileName, body: body, keys: "enter apply  esc back"}
	case modalAdminPassword:
		body := []string{
			m.formLine("new password", maskSecret(m.adminPassword), 0, true),
			m.formLine("confirm", maskSecret(m.adminPasswordConfirm), 1, true),
		}
		if m.adminPasswordErr != nil {
			body = append(body, errorStyle.Render(m.adminPasswordErr.Error()))
		} else {
			body = append(body, mutedStyle.Render("Changes the admin login; not admin_passwd."))
		}
		return modalSpec{title: "Admin password · " + m.taskProfileName + " / " + m.taskDatabaseName, body: body, keys: "tab field  enter apply  esc cancel"}
	}
	return modalSpec{}
}

// configHint is the single line under a config form: the error if any,
// otherwise a muted explanation.
func (m *Model) configHint(text string) string {
	if m.configFormErr != nil {
		return errorStyle.Render(m.configFormErr.Error())
	}
	return mutedStyle.Render(text)
}

// modalView draws the modal: a dim rounded border carrying the title on top
// and the key hints along the bottom, with no padding rows.
func (m *Model) modalView() string {
	spec := m.modalSpec()
	maxWidth := max(12, min(modalMaxWidth, m.width-2))
	minWidth := min(modalMinWidth, maxWidth)

	widest := lipgloss.Width(spec.title) + 6
	for _, line := range spec.body {
		widest = max(widest, lipgloss.Width(line)+4)
	}
	width := min(max(widest, minWidth), maxWidth)
	inner := width - 4

	body := make([]string, 0, len(spec.body))
	for _, line := range spec.body {
		wrapped := lipgloss.NewStyle().Width(inner).Render(line)
		body = append(body, strings.Split(wrapped, "\n")...)
	}
	if limit := max(1, m.height-2); len(body) > limit {
		body = append(body[:limit-1], mutedStyle.Render("…"))
	}

	border := func(value string) string { return dimStyle.Render(value) }
	lines := make([]string, 0, len(body)+2)
	lines = append(lines, modalTopBorder(spec.title, width))
	for _, line := range body {
		pad := strings.Repeat(" ", max(0, inner-lipgloss.Width(line)))
		lines = append(lines, border("│")+" "+line+pad+" "+border("│"))
	}
	lines = append(lines, modalBottomBorder(spec.keys, width))
	return strings.Join(lines, "\n")
}

// modalTopBorder embeds the title in the top border; context after " · " is
// muted. A title that does not fit is cut, never wrapped.
func modalTopBorder(title string, width int) string {
	name, context, hasContext := strings.Cut(title, " · ")
	room := width - 6
	name = ansi.Truncate(name, room, "…")
	label := titleStyle.Render(name)
	if hasContext {
		if left := room - lipgloss.Width(name) - 3; left > 1 {
			label += mutedStyle.Render(" · " + ansi.Truncate(context, left, "…"))
		}
	}
	fill := max(0, width-5-lipgloss.Width(label))
	return dimStyle.Render("╭─ ") + label + dimStyle.Render(" "+strings.Repeat("─", fill)+"╮")
}

// modalBottomBorder embeds the key hints in the bottom border, dropping whole
// hints from the end when they do not fit.
func modalBottomBorder(keys string, width int) string {
	room := width - 6
	hints := ""
	for _, part := range strings.Split(keys, "  ") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, desc, _ := strings.Cut(part, " ")
		next := keyHint{key, desc}.render()
		if hints != "" {
			next = "  " + next
		}
		if hints != "" && lipgloss.Width(hints)+lipgloss.Width(next) > room {
			break
		}
		hints += next
	}
	hints = ansi.Truncate(hints, room, "")
	fill := max(0, width-5-lipgloss.Width(hints))
	if hints == "" {
		return dimStyle.Render("╰" + strings.Repeat("─", width-2) + "╯")
	}
	return dimStyle.Render("╰─ ") + hints + dimStyle.Render(" "+strings.Repeat("─", fill)+"╯")
}

// dimBackdrop strips the colours from the view behind a modal and paints every
// cell in one dark zinc, so the dashboard stays visible without competing.
func dimBackdrop(background string, width int) []string {
	lines := strings.Split(background, "\n")
	style := lipgloss.NewStyle().Foreground(colorBackdrop)
	for index, line := range lines {
		plain := ansi.Truncate(ansi.Strip(line), width, "")
		plain += strings.Repeat(" ", max(0, width-ansi.StringWidth(plain)))
		lines[index] = style.Render(plain)
	}
	return lines
}

func (m *Model) modalOverlay(background string) string {
	base := dimBackdrop(background, m.width)
	modalLines := strings.Split(m.modalView(), "\n")
	modalWidth := lipgloss.Width(modalLines[0])
	left := max(0, (m.width-modalWidth)/2)
	top := max(0, (m.height-len(modalLines))/2)
	for row := range base {
		if row < top || row >= top+len(modalLines) {
			continue
		}
		base[row] = ansi.Cut(base[row], 0, left) + modalLines[row-top] + ansi.Cut(base[row], left+modalWidth, m.width)
	}
	return m.fitView(strings.Join(base, "\n"))
}

// modalListBudget is how many list rows fit in a modal at the current height.
func (m *Model) modalListBudget() int {
	return max(3, m.height-9)
}
