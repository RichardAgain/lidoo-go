package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type listFilter struct {
	query string
}

func (f listFilter) indices(count int, label func(int) string) []int {
	indices := make([]int, 0, count)
	query := strings.ToLower(f.query)
	for index := 0; index < count; index++ {
		if strings.Contains(strings.ToLower(label(index)), query) {
			indices = append(indices, index)
		}
	}
	return indices
}

func (f *listFilter) update(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "backspace":
		f.query = removeLastRune(f.query)
	case "ctrl+u":
		f.query = ""
	default:
		value := keyInsertText(msg)
		if value == "" {
			return false
		}
		f.query += value
	}
	return true
}

func containsIndex(indices []int, selected int) bool {
	for _, index := range indices {
		if index == selected {
			return true
		}
	}
	return false
}

func selectFirstIndex(selected *int, indices []int) {
	*selected = -1
	if len(indices) > 0 {
		*selected = indices[0]
	}
}

func moveFilteredIndex(selected *int, indices []int, delta int) {
	for position, index := range indices {
		if index == *selected {
			position += delta
			if position >= 0 && position < len(indices) {
				*selected = indices[position]
			}
			return
		}
	}
	selectFirstIndex(selected, indices)
}

func (m *Model) selectIndices() []int {
	return m.selectFilter.indices(len(m.selectItems), func(index int) string {
		return m.selectItems[index].Label
	})
}

func (m *Model) filePickerIndices() []int {
	return m.filePickerFilter.indices(len(m.filePickerItems), func(index int) string {
		return m.filePickerItems[index].Name
	})
}

func (m *Model) filteredListLines(indices []int, selected int, label func(int) string) []string {
	if len(indices) == 0 {
		return []string{mutedStyle.Render("No matches")}
	}
	position := 0
	for offset, index := range indices {
		if index == selected {
			position = offset
			break
		}
	}
	budget := m.modalListBudget() - 1
	if budget < 1 {
		budget = 1
	}
	start := windowStart(len(indices), position, budget)
	end := min(start+budget, len(indices))
	var lines []string
	if start > 0 {
		lines = append(lines, mutedStyle.Render(fmt.Sprintf("  ↑ %d more", start)))
	}
	for _, index := range indices[start:end] {
		if index == selected {
			lines = append(lines, activeStyle.Render("▌ "+label(index)))
		} else {
			lines = append(lines, "  "+textStyle.Render(label(index)))
		}
	}
	if end < len(indices) {
		lines = append(lines, mutedStyle.Render(fmt.Sprintf("  ↓ %d more", len(indices)-end)))
	}
	return lines
}

func (m *Model) filePickerModalLines() []string {
	title := "Restore source · " + m.taskDatabaseName
	hint := "↑/↓ move  type filter  enter open/select  ctrl+h/← up  esc cancel"
	empty := "No folders or .zip/.dump files here"
	if m.filePickerDirectory {
		title = "Backup destination folder"
		hint = "↑/↓ move  type filter  enter open  ctrl+s use folder  ctrl+h/← up  esc back"
		empty = "No folders here"
	}
	lines := []string{titleStyle.Render(title), mutedStyle.Render(m.filePickerPath), "Filter: " + m.filePickerFilter.query + "▏", ""}
	switch m.filePickerPhase {
	case phaseLoading:
		lines = append(lines, mutedStyle.Render("Loading…"))
	case phaseError:
		lines = append(lines, errorStyle.Render(errorText(m.filePickerErr)))
	case phaseEmpty:
		lines = append(lines, mutedStyle.Render(empty))
	case phaseReady:
		lines = append(lines, m.filteredListLines(m.filePickerIndices(), m.filePickerIndex, func(index int) string {
			entry := m.filePickerItems[index]
			if entry.Dir {
				return entry.Name + "/"
			}
			return entry.Name
		})...)
	default:
		lines = append(lines, mutedStyle.Render("not loaded"))
	}
	return append(lines, "", modalKeyHints(hint))
}

func (m *Model) selectModalLines() []string {
	lines := []string{titleStyle.Render(m.selectTitle), "Filter: " + m.selectFilter.query + "▏"}
	switch m.selectPhase {
	case phaseLoading:
		lines = append(lines, mutedStyle.Render("Loading options..."))
	case phaseEmpty:
		lines = append(lines, mutedStyle.Render("Nothing to choose"))
	case phaseError:
		lines = append(lines, errorStyle.Render(errorText(m.selectErr)))
	case phaseReady:
		lines = append(lines, m.filteredListLines(m.selectIndices(), m.selectIndex, func(index int) string {
			item := m.selectItems[index]
			label := item.Label
			if item.Hint != "" {
				label += "  " + mutedStyle.Render(item.Hint)
			}
			return label
		})...)
	default:
		lines = append(lines, mutedStyle.Render("Options are not available"))
	}
	if m.selectErr != nil && m.selectPhase == phaseReady {
		lines = append(lines, errorStyle.Render(m.selectErr.Error()))
	}
	return append(lines, "", modalKeyHints("↑/↓ move  type filter  enter select  esc cancel"))
}

func (m *Model) startBackupDirectoryPicker() tea.Cmd {
	path := filepath.Dir(m.backupDestination)
	if m.backupDestination == "" {
		path = "."
	}
	if !filepath.IsAbs(path) {
		if dir, err := os.Getwd(); err == nil {
			path = filepath.Join(dir, path)
		}
	}
	m.filePickerDirectory = true
	return m.openFilePicker(path)
}

func (m *Model) chooseBackupDirectory() {
	filename := filepath.Base(m.backupDestination)
	if m.backupDestination == "" || strings.HasSuffix(m.backupDestination, string(filepath.Separator)) {
		filename = m.taskDatabaseName
		if m.backupFormat != "folder" {
			filename += "." + m.backupFormat
		}
	}
	m.backupDestination = filepath.Join(m.filePickerPath, filename)
	m.modal = modalBackupDatabase
	m.formField = 0
}
