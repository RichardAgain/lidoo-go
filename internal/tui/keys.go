package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// keyHint pairs a key with what it does, for the footer and the key overlay.
type keyHint struct {
	key  string
	desc string
}

func (hint keyHint) render() string {
	return keyStyle.Render(hint.key) + " " + mutedStyle.Render(hint.desc)
}

// footerHints lists the few keys that matter for the focused pane and tab, most
// useful first, so the footer can drop hints from the end when it is narrow.
func (m *Model) footerHints() []keyHint {
	if m.selectedProfileName() == "" {
		return []keyHint{{"n", "new"}, {"ctrl+r", "refresh"}, {"q", "quit"}}
	}
	if m.onProfiles() {
		toggle := keyHint{"space", "start"}
		if m.profileIsRunning() {
			toggle.desc = "stop"
		}
		return []keyHint{{"↑↓", "profile"}, {"enter", "open tabs"}, toggle, {"o", "browser"}, {"n", "new"}, {"d", "remove"}}
	}
	switch m.tab {
	case tabDatabases:
		if m.selectedDatabase() == nil {
			return []keyHint{{"i", "new"}, {"R", "restore"}, {"esc", "back"}}
		}
		return []keyHint{{"↑↓", "database"}, {"u", "update"}, {"b", "backup"}, {"R", "restore"}, {"p", "psql"}, {"esc", "back"}}
	case tabAddons:
		attach := keyHint{"enter", "attach"}
		if addon := m.selectedAddon(); addon != nil && addonAttached(*addon, m.selectedProfileName()) {
			attach.desc = "detach"
		}
		if m.selectedAddon() == nil {
			return []keyHint{{"c", "clone"}, {"w", "worktree"}, {"esc", "back"}}
		}
		return []keyHint{{"↑↓", "add-on"}, attach, {"c", "clone"}, {"w", "worktree"}, {"f", "fetch"}, {"esc", "back"}}
	case tabInfo:
		return []keyHint{{"[ ]", "tab"}, {"K J", "profile"}, {"o", "browser"}, {"esc", "back"}}
	case tabConfig:
		return []keyHint{{"↑↓", "setting"}, {"enter", "change"}, {"R", "recreate"}, {"esc", "back"}}
	}
	return []keyHint{{"↑↓", "scroll"}, {"/", "search"}, {"L", "fullscreen"}, {"D", "database"}, {"esc", "back"}}
}

// footerLine is the one-row footer: contextual hints on the left and the key
// reference on the right.
func (m *Model) footerLine(width int) string {
	if message := m.statusMessage(); message != "" {
		pinned := keyHint{"?", "all keys"}.render() + " "
		room := max(0, width-lipgloss.Width(pinned)-3)
		left := " " + ansi.Truncate(message, room, "…")
		return left + strings.Repeat(" ", max(1, width-lipgloss.Width(left)-lipgloss.Width(pinned))) + pinned
	}
	return hintsLine(width, m.footerHints(), keyHint{"?", "all keys"})
}

// hintsLine fits hints on one row of the given width, dropping whole hints
// from the end rather than cutting one. Pinned hints stay right-aligned.
func hintsLine(width int, hints []keyHint, pinned ...keyHint) string {
	right := ""
	for index, hint := range pinned {
		if index > 0 {
			right += "   "
		}
		right += hint.render()
	}
	if right != "" {
		right += " "
	}
	room := width - 1 - lipgloss.Width(right) - 2
	left := ""
	for index, hint := range hints {
		next := hint.render()
		if index > 0 {
			next = "   " + next
		}
		if index > 0 && lipgloss.Width(left)+lipgloss.Width(next) > room {
			break
		}
		left += next
	}
	left = ansi.Truncate(" "+left, max(0, width-lipgloss.Width(right)-1), "")
	return left + strings.Repeat(" ", max(1, width-lipgloss.Width(left)-lipgloss.Width(right))) + right
}

type keyGroup struct {
	name  string
	hints []keyHint
	note  string
}

var keyGroups = []keyGroup{
	{name: "Navigate", hints: []keyHint{
		{"↑↓ j k", "move"}, {"enter tab → l", "open tabs"}, {"esc tab ← h", "back"}, {"1-5", "open a tab"},
		{"[ ]", "step tabs"}, {"K J", "other profile"}, {"ctrl+v", "paste in inputs"}, {"ctrl+r", "refresh"}, {"q", "quit"},
	}},
	{name: "Profile", hints: []keyHint{
		{"space", "start/stop"}, {"x", "start"}, {"s", "stop"}, {"r", "restart"}, {"R", "recreate"},
		{"n c", "new"}, {"d", "remove"}, {"e", "config"}, {"o", "browser"},
	}, note: "Saved without starting; R applies config changes."},
	{name: "Databases", hints: []keyHint{
		{"i", "new"}, {"u", "update"}, {"U", "update all"}, {"b", "backup"}, {"R", "restore"},
		{"p", "psql"}, {"a", "admin password"}, {"d", "drop"},
	}},
	{name: "Add-ons", hints: []keyHint{
		{"enter", "attach/detach"}, {"c", "clone"}, {"w", "worktree"}, {"f", "fetch"}, {"p", "pull"}, {"x", "remove"},
	}, note: "Missing worktree branches are created."},
	{name: "Logs", hints: []keyHint{
		{"↑↓ pgup pgdn", "scroll"}, {"/", "search"}, {"L", "fullscreen"}, {"D", "database"},
	}},
	{name: "Tasks", hints: []keyHint{{"c", "cancel"}, {"t", "expand/collapse last task"}}},
}

const keyGroupNameWidth = 11

// keysModalSpec lays the key reference out as group names beside wrapped
// key/description pairs. Notes are dropped when the terminal is too short.
func (m *Model) keysModalSpec() modalSpec {
	itemWidth := min(m.width-4, 72) - 6 - keyGroupNameWidth
	build := func(withNotes bool) []string {
		var lines []string
		for _, group := range keyGroups {
			rows := wrapHints(group.hints, itemWidth)
			for index, row := range rows {
				name := strings.Repeat(" ", keyGroupNameWidth)
				if index == 0 {
					name = activeStyle.Render(group.name) + strings.Repeat(" ", keyGroupNameWidth-len(group.name))
				}
				lines = append(lines, name+row)
			}
			if withNotes && group.note != "" {
				lines = append(lines, strings.Repeat(" ", keyGroupNameWidth)+mutedStyle.Render(group.note))
			}
		}
		return lines
	}
	body := build(true)
	if len(body)+2 > m.height {
		body = build(false)
	}
	return modalSpec{title: "Keys", body: body, keys: "any key closes"}
}

// wrapHints packs hints into rows no wider than width.
func wrapHints(hints []keyHint, width int) []string {
	var rows []string
	current := ""
	for _, hint := range hints {
		item := hint.render()
		if current != "" && lipgloss.Width(current)+2+lipgloss.Width(item) > width {
			rows = append(rows, current)
			current = ""
		}
		if current != "" {
			current += "  "
		}
		current += item
	}
	if current != "" {
		rows = append(rows, current)
	}
	return rows
}
