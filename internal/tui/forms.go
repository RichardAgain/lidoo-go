package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// colorBoxIdle is the border of an input that does not have focus.
const colorBoxIdle = lipgloss.Color("#3f3f46")

type fieldKind uint8

const (
	fieldText fieldKind = iota
	fieldToggle
	fieldChoice
	fieldSelect
)

// formField describes one row of a form. Values stay in the model's own string
// and bool fields; the field only points at them.
type formField struct {
	kind        fieldKind
	label       string
	text        *string
	placeholder string
	secret      bool
	on          *bool
	caption     string // toggle: what is switched on
	options     []string
	current     func() string // choice: the active option
	choose      func(string)  // choice: select an option
	sel         *selectField
}

// selectField is an inline radio list. A Custom… entry reveals a text input
// under the list; the typed text becomes the value.
type selectField struct {
	items  []selectItem
	index  int
	custom string
}

func (s *selectField) isCustom() bool {
	return s.index >= 0 && s.index < len(s.items) && s.items[s.index].Value == selectCustomValue
}

// value is the chosen item's value, or the trimmed custom text.
func (s *selectField) value() string {
	if s.isCustom() {
		return strings.TrimSpace(s.custom)
	}
	if s.index < 0 || s.index >= len(s.items) {
		return ""
	}
	return s.items[s.index].Value
}

func (s *selectField) move(delta int) {
	if len(s.items) == 0 {
		return
	}
	s.index = min(max(s.index+delta, 0), len(s.items)-1)
}

// selectFormFields is the select row plus, when Custom… is chosen, the input
// that sits directly under the list.
func selectFormFields(label string, sel *selectField, placeholder string) []formField {
	fields := []formField{{kind: fieldSelect, label: label, sel: sel}}
	if sel.isCustom() {
		fields = append(fields, textField("", &sel.custom, placeholder))
	}
	return fields
}

func textField(label string, value *string, placeholder string) formField {
	return formField{kind: fieldText, label: label, text: value, placeholder: placeholder}
}

func secretField(label string, value *string) formField {
	return formField{kind: fieldText, label: label, text: value, secret: true}
}

func toggleField(label string, value *bool, caption string) formField {
	return formField{kind: fieldToggle, label: label, on: value, caption: caption}
}

// formFields lists the fields of the open form, in tab order.
func (m *Model) formFields() []formField {
	switch m.modal {
	case modalInitDatabase:
		return []formField{
			textField("Database", &m.initDatabaseName, "name"),
			textField("Modules", &m.initModules, "base,sale"),
		}
	case modalBackupDatabase:
		return []formField{
			textField("Destination", &m.backupDestination, "backups/<database>_<date>.zip"),
			{kind: fieldChoice, label: "Format", options: []string{"zip", "dump", "folder"},
				current: func() string { return m.backupFormat }, choose: func(value string) { m.backupFormat = value }},
			toggleField("Filestore", &m.backupFilestore, "include"),
			toggleField("Overwrite", &m.backupForce, "replace an existing file"),
		}
	case modalRestoreDatabase:
		return []formField{
			textField("Destination", &m.restoreDestination, "new database name"),
			textField("Source", &m.restoreSource, "path to a .zip or .dump"),
			{kind: fieldChoice, label: "Mode", options: []string{"copy", "move"},
				current: func() string { return restoreMode(m.restoreCopy) },
				choose:  func(value string) { m.restoreCopy = value == "copy" }},
			toggleField("Overwrite", &m.restoreForce, "replace the destination"),
			toggleField("Neutralize", &m.restoreNeutralize, "disable mail and crons"),
			textField("Jobs", &m.restoreJobs, "1"),
		}
	case modalCloneAddon:
		return []formField{
			textField("Name", &m.cloneAddonName, "add-on name"),
			textField("Git URL", &m.cloneAddonURL, "git@github.com:org/repo.git"),
			textField("Depth", &m.cloneAddonDepth, "0 = full"),
			textField("Branch", &m.cloneAddonBranch, "optional"),
		}
	case modalWorktreeAddon:
		return []formField{
			textField("Name", &m.worktreeAddonName, "worktree name"),
			textField("Branch", &m.worktreeAddonBranch, "branch"),
		}
	case modalCreateProfile:
		return []formField{
			textField("Name", &m.createProfileName, "profile name"),
			textField("Odoo version", &m.createProfileVersion, "18"),
		}
	case modalConfigPattern:
		return []formField{textField("Pattern", &m.configPatternDraft, "^smoke__.*$")}
	case modalConfigPassword:
		return []formField{secretField("Password", &m.configPasswordDraft)}
	case modalConfigLANPort:
		return []formField{textField("Host port", &m.configLANPortDraft, "0 disables")}
	case modalAdminPassword:
		return []formField{
			secretField("New password", &m.adminPassword),
			secretField("Confirm", &m.adminPasswordConfirm),
		}
	}
	return nil
}

func (m *Model) isFormModal() bool {
	return len(m.formFields()) > 0
}

func (m *Model) formFieldCount() int {
	return len(m.formFields())
}

func (m *Model) moveFormField(delta int) {
	count := m.formFieldCount()
	if count == 0 {
		return
	}
	m.formField = (m.formField + delta + count) % count
}

// focusedField is the field that has the keyboard, or nil.
func (m *Model) focusedField() *formField {
	fields := m.formFields()
	if m.formField < 0 || m.formField >= len(fields) {
		return nil
	}
	return &fields[m.formField]
}

// formText is the string behind the focused text field, or nil.
func (m *Model) formText() *string {
	if field := m.focusedField(); field != nil && field.kind == fieldText {
		return field.text
	}
	return nil
}

// activateFormField flips the focused toggle or advances the focused choice.
func (m *Model) activateFormField(delta int) bool {
	field := m.focusedField()
	if field == nil {
		return false
	}
	switch field.kind {
	case fieldToggle:
		*field.on = !*field.on
		return true
	case fieldChoice:
		current := field.current()
		for index, option := range field.options {
			if option == current {
				field.choose(field.options[(index+delta+len(field.options))%len(field.options)])
				return true
			}
		}
		field.choose(field.options[0])
		return true
	}
	return false
}

// newFormInput builds the single text input that edits whichever text field is
// focused.
func newFormInput() textinput.Model {
	input := textinput.New()
	input.Prompt = ""
	input.TextStyle = textStyle
	input.PlaceholderStyle = mutedStyle
	input.Cursor.SetMode(cursor.CursorStatic)
	input.EchoCharacter = '•'
	input.Focus()
	return input
}

// ensureFormInput points the text input at the focused field. The field's own
// string stays the source of truth; the input adds the caret and editing.
func (m *Model) ensureFormInput() {
	field := m.focusedField()
	if field == nil || field.kind != fieldText {
		m.formInputKey = formInputKey{}
		return
	}
	key := formInputKey{modal: m.modal, field: m.formField, valid: true}
	if m.formInputKey != key || m.formInput.Value() != *field.text {
		m.formInput.SetValue(*field.text)
		m.formInput.CursorEnd()
	}
	m.formInputKey = key
	m.formInput.Placeholder = field.placeholder
	m.formInput.EchoMode = textinput.EchoNormal
	if field.secret {
		m.formInput.EchoMode = textinput.EchoPassword
	}
}

type formInputKey struct {
	modal modalMode
	field int
	valid bool
}

// editFormText sends a key to the text input and stores the result.
func (m *Model) editFormText(msg tea.KeyMsg) {
	m.ensureFormInput()
	target := m.formText()
	if target == nil {
		return
	}
	if msg.Paste {
		// Line breaks and tabs in a paste must not become spaces or submit.
		msg.Runes = []rune(sanitizePaste(string(msg.Runes)))
	}
	if msg.Alt && msg.Type == tea.KeyRunes && (len(msg.Runes) == 0 || !strings.ContainsRune("bfd", msg.Runes[0])) {
		return // an alt chord is not text; only alt+b/f/d move by word
	}
	m.formInput, _ = m.formInput.Update(msg)
	*target = m.formInput.Value()
}

// insertFormText inserts text at the caret of the focused text field.
func (m *Model) insertFormText(text string) {
	m.ensureFormInput()
	target := m.formText()
	if target == nil {
		return
	}
	runes := []rune(m.formInput.Value())
	position := min(m.formInput.Position(), len(runes))
	inserted := []rune(text)
	runes = append(runes[:position], append(inserted, runes[position:]...)...)
	m.formInput.SetValue(string(runes))
	m.formInput.SetCursor(position + len(inserted))
	*target = m.formInput.Value()
}

func (m *Model) clearFormErrors() {
	m.addonFormErr = nil
	m.configFormErr = nil
	m.restoreErr = nil
	m.adminPasswordErr = nil
	m.createFormErr = nil
}

// updateFormKey handles every key for the form modals.
func (m *Model) updateFormKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc":
		m.modal = modalNone
		return m, nil
	case "enter":
		return m, m.submitForm()
	case "tab":
		m.moveFormField(1)
		return m, nil
	case "shift+tab":
		m.moveFormField(-1)
		return m, nil
	case "ctrl+o":
		if m.modal == modalBackupDatabase {
			return m, m.startBackupDirectoryPicker()
		}
	}
	field := m.focusedField()
	if field == nil {
		return m, nil
	}
	switch field.kind {
	case fieldToggle:
		if key == " " {
			m.activateFormField(1)
		}
	case fieldChoice:
		switch key {
		case " ", "right":
			m.activateFormField(1)
		case "left":
			m.activateFormField(-1)
		}
	case fieldSelect:
		switch key {
		case "up":
			field.sel.move(-1)
		case "down":
			field.sel.move(1)
		}
		m.clearFormErrors()
	default:
		m.clearFormErrors()
		m.editFormText(msg)
	}
	return m, nil
}

func (m *Model) submitForm() tea.Cmd {
	switch m.modal {
	case modalInitDatabase, modalBackupDatabase, modalRestoreDatabase:
		return m.submitDatabaseForm()
	case modalCloneAddon, modalWorktreeAddon:
		return m.submitAddonForm()
	}
	return m.submitProfileForm()
}

// textBox draws a text field as a three-row rounded box with the label on the
// middle row. content is already styled and exactly the inner width.
func textBox(label string, labelWidth int, focused bool, content string, boxWidth int) []string {
	border := lipgloss.NewStyle().Foreground(colorBoxIdle)
	labelStyle := secondStyle
	if focused {
		border = lipgloss.NewStyle().Foreground(colorSecond)
		labelStyle = activeStyle
	}
	blank := strings.Repeat(" ", labelWidth)
	middleLabel := labelStyle.Render(label) + strings.Repeat(" ", max(0, labelWidth-ansi.StringWidth(label)))
	horizontal := strings.Repeat("─", max(0, boxWidth-2))
	return []string{
		blank + border.Render("╭"+horizontal+"╮"),
		middleLabel + border.Render("│") + " " + content + " " + border.Render("│"),
		blank + border.Render("╰"+horizontal+"╯"),
	}
}

// formBody renders the fields for a modal body of the given inner width.
func (m *Model) formBody(fields []formField, inner int) []string {
	m.ensureFormInput()
	labelWidth := 0
	for _, field := range fields {
		labelWidth = max(labelWidth, ansi.StringWidth(field.label))
	}
	labelWidth += 2
	boxWidth := max(8, inner-labelWidth)
	textWidth := boxWidth - 4

	var lines []string
	for index, field := range fields {
		focused := index == m.formField
		switch field.kind {
		case fieldText:
			lines = append(lines, textBox(field.label, labelWidth, focused, m.fieldContent(field, focused, textWidth), boxWidth)...)
		case fieldSelect:
			lines = append(lines, m.selectRows(field, labelWidth, focused)...)
		default:
			lines = append(lines, m.optionRow(field, labelWidth, focused))
		}
	}
	return lines
}

// fieldContent is the inside of a text box: the live input when focused, the
// stored value or a muted placeholder otherwise.
func (m *Model) fieldContent(field formField, focused bool, width int) string {
	var content string
	switch {
	case focused:
		m.formInput.Width = max(1, width-1)
		content = m.formInput.View()
	case *field.text == "":
		content = mutedStyle.Render(ansi.Truncate(field.placeholder, width, "…"))
	default:
		value := *field.text
		if field.secret {
			value = maskSecret(value)
		}
		content = textStyle.Render(ansi.Truncate(value, width, "…"))
	}
	content = ansi.Truncate(content, width, "")
	return content + strings.Repeat(" ", max(0, width-ansi.StringWidth(content)))
}

// optionRow renders a toggle or choice on one row.
func (m *Model) optionRow(field formField, labelWidth int, focused bool) string {
	labelStyle := secondStyle
	if focused {
		labelStyle = activeStyle
	}
	label := labelStyle.Render(field.label) + strings.Repeat(" ", max(0, labelWidth-ansi.StringWidth(field.label)))
	switch field.kind {
	case fieldToggle:
		box, style := "[ ]", secondStyle
		if *field.on {
			box = "[x]"
		}
		if focused {
			style = activeStyle
		}
		return label + style.Render(box+" "+field.caption)
	default:
		current := field.current()
		segments := make([]string, 0, len(field.options))
		for _, option := range field.options {
			style := secondStyle
			if option == current {
				style = lipgloss.NewStyle().Background(colorRule).Foreground(colorText).Bold(true)
				if focused {
					style = style.Foreground(colorWhite)
				}
			}
			segments = append(segments, style.Render(" "+option+" "))
		}
		return label + strings.Join(segments, " ")
	}
}

// selectRows renders an inline radio list; the label sits on the first row.
func (m *Model) selectRows(field formField, labelWidth int, focused bool) []string {
	labelStyle := secondStyle
	if focused {
		labelStyle = activeStyle
	}
	label := labelStyle.Render(field.label) + strings.Repeat(" ", max(0, labelWidth-ansi.StringWidth(field.label)))
	blank := strings.Repeat(" ", labelWidth)
	items := field.sel.items
	if len(items) == 0 {
		return []string{label + mutedStyle.Render("loading…")}
	}
	nameWidth := 0
	for _, item := range items {
		nameWidth = max(nameWidth, ansi.StringWidth(item.Label))
	}
	rows := make([]string, 0, len(items))
	for index, item := range items {
		prefix := blank
		if index == 0 {
			prefix = label
		}
		radio, style := "○", secondStyle
		if index == field.sel.index {
			radio, style = "●", textStyle
			if focused {
				style = activeStyle
			}
		}
		row := prefix + style.Render(radio+" "+item.Label+strings.Repeat(" ", nameWidth-ansi.StringWidth(item.Label)))
		if item.Hint != "" {
			row += "   " + mutedStyle.Render(item.Hint)
		}
		rows = append(rows, row)
	}
	return rows
}
