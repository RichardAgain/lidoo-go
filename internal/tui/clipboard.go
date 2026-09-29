package tui

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
)

// ClipboardContentMsg carries the result of an explicit ctrl+v paste.
type ClipboardContentMsg struct {
	Text string
	Err  error
}

// clipboardCommands are tried in order; the first tool actually installed
// answers. Using the system tools keeps the project dependency free and only
// spawns a process when the user asks for a paste.
var clipboardCommands = [][]string{
	{"wl-paste", "--no-newline"},
	{"pbpaste"},
	{"xclip", "-selection", "clipboard", "-o"},
	{"xsel", "--clipboard", "--output"},
}

// ReadClipboardCmd reads the system clipboard for an explicit ctrl+v paste.
func ReadClipboardCmd() tea.Cmd {
	return func() tea.Msg {
		failed := make([]string, 0, len(clipboardCommands))
		for _, candidate := range clipboardCommands {
			path, err := exec.LookPath(candidate[0])
			if err != nil {
				continue
			}
			out, err := exec.Command(path, candidate[1:]...).Output()
			if err != nil {
				failed = append(failed, candidate[0])
				continue
			}
			return ClipboardContentMsg{Text: sanitizePaste(string(out))}
		}
		if len(failed) > 0 {
			return ClipboardContentMsg{Err: fmt.Errorf("%s could not read the clipboard", strings.Join(failed, ", "))}
		}
		return ClipboardContentMsg{Err: errors.New("no clipboard tool found; install wl-clipboard, xclip, or xsel")}
	}
}

// keyInsertText is the text a key contributes to a text field. Regular typing
// arrives as one printable rune, but a bracketed paste arrives as a single
// message carrying every pasted rune, so multi-rune input must be accepted
// too. Alt-modified keys stay out because they carry a modifier the form does
// not expect.
func keyInsertText(msg tea.KeyMsg) string {
	if msg.Alt || (msg.Type != tea.KeyRunes && msg.Type != tea.KeySpace) {
		return ""
	}
	return sanitizePaste(string(msg.Runes))
}

// sanitizePaste keeps the printable characters of pasted text and drops the
// control characters a paste can carry (newlines, tabs, carriage returns) that
// would otherwise submit or skip through a form.
func sanitizePaste(value string) string {
	var builder strings.Builder
	for _, character := range value {
		if unicode.IsPrint(character) {
			builder.WriteRune(character)
		}
	}
	return builder.String()
}
