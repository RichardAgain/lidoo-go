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
	// render builds the body for the final inner width; wantInner is the width
	// it would like. Forms use them so the boxes can fill the modal.
	render    func(inner int) []string
	wantInner int
}

const formBoxWidth = 36

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
		return m.formSpec("Initialize database · "+m.taskProfileName, "", "enter run")
	case modalBackupDatabase:
		return m.formSpec("Backup · "+m.taskProfileName+" / "+m.taskDatabaseName, "", "enter run")
	case modalRestoreDatabase:
		hint := mutedStyle.Render("copy: new database · move: replace destination")
		if m.restoreErr != nil {
			hint = errorStyle.Render(m.restoreErr.Error())
		}
		return m.formSpec("Restore database · "+m.taskProfileName, hint, "enter run")
	case modalCloneAddon:
		hint := ""
		if m.addonFormErr != nil {
			hint = errorStyle.Render(m.addonFormErr.Error())
		}
		return m.formSpec("Clone and register add-on", hint, "enter clone")
	case modalWorktreeAddon:
		hint := mutedStyle.Render("Missing branches are created.")
		if m.addonFormErr != nil {
			hint = errorStyle.Render(m.addonFormErr.Error())
		}
		return m.formSpec("Create worktree · "+m.worktreeAddonSource, hint, "enter create")
	case modalKeys:
		return m.keysModalSpec()
	case modalSelect:
		return m.selectModalSpec()
	case modalFilePicker:
		return m.filePickerModalSpec()
	case modalCreateProfile:
		hint := mutedStyle.Render("space starts the profile after creation")
		if m.createFormErr != nil {
			hint = errorStyle.Render(m.createFormErr.Error())
		}
		return m.formSpec("New profile", hint, "enter create")
	case modalConfigPattern:
		return m.formSpec("Database filter pattern · "+m.configProfileName, m.configHint("Matches physical database names."), "enter apply")
	case modalConfigPassword:
		return m.formSpec("Odoo master password · "+m.configProfileName, m.configHint("admin_passwd for /web/database; not the login password"), "enter apply")
	case modalConfigLANPort:
		return m.formSpec("LAN port · "+m.configProfileName, m.configHint("0 disables LAN access; R recreates to apply"), "enter apply")
	case modalAdminPassword:
		hint := mutedStyle.Render("Changes the admin login; not admin_passwd.")
		if m.adminPasswordErr != nil {
			hint = errorStyle.Render(m.adminPasswordErr.Error())
		}
		return m.formSpec("Admin password · "+m.taskProfileName+" / "+m.taskDatabaseName, hint, "enter apply")
	}
	return modalSpec{}
}

// formSpec builds a form modal: the fields, one hint or error line, and key
// hints that follow the focused field.
func (m *Model) formSpec(title, hint, submit string) modalSpec {
	fields := m.formFields()
	labelWidth := 0
	for _, field := range fields {
		labelWidth = max(labelWidth, lipgloss.Width(field.label))
	}
	return modalSpec{
		title:     title,
		wantInner: labelWidth + 2 + formBoxWidth,
		render: func(inner int) []string {
			lines := m.formBody(fields, inner)
			if hint != "" {
				lines = append(lines, hint)
			}
			return lines
		},
		keys: m.formKeys(submit),
	}
}

// formKeys are the bottom-border hints for the focused field.
func (m *Model) formKeys(submit string) string {
	keys := "tab next"
	if field := m.focusedField(); field != nil {
		switch field.kind {
		case fieldToggle:
			keys += "  space toggle"
		case fieldChoice:
			keys += "  space change"
		case fieldSelect:
			keys += "  ↑↓ choose"
		}
	}
	if m.modal == modalBackupDatabase {
		keys += "  ctrl+o folder"
	}
	return keys + "  " + submit + "  esc cancel"
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

	widest := max(lipgloss.Width(spec.title)+6, spec.wantInner+4)
	for _, line := range spec.body {
		widest = max(widest, lipgloss.Width(line)+4)
	}
	width := min(max(widest, minWidth), maxWidth)
	inner := width - 4
	if spec.render != nil {
		spec.body = spec.render(inner)
	}

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
