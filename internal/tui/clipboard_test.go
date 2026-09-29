package tui

import (
	"context"
	"errors"
	"testing"

	"github.com/charmbracelet/bubbletea"
)

func TestBracketedPasteFillsGitURL(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.modal = modalCloneAddon
	model.formField = 1
	model.cloneAddonName = "lidoo"

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("https://github.com/acme/addon.git\n"), Paste: true}
	if _, cmd := model.updateKey(msg); cmd != nil {
		t.Fatal("pasting a URL should not queue a task")
	}
	if model.cloneAddonURL != "https://github.com/acme/addon.git" {
		t.Fatalf("git url = %q", model.cloneAddonURL)
	}
	if model.cloneAddonName != "lidoo" {
		t.Fatalf("paste leaked into another field: %q", model.cloneAddonName)
	}
}

func TestTypedRuneStillInsertsOneCharacter(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.modal = modalCloneAddon
	model.formField = 1

	if _, cmd := model.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")}); cmd != nil {
		t.Fatal("typing should not queue a task")
	}
	if model.cloneAddonURL != "a" {
		t.Fatalf("git url = %q, want a", model.cloneAddonURL)
	}
}

func TestAltKeyIsNotInsertedAsText(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.modal = modalCloneAddon
	model.formField = 1

	model.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e"), Alt: true})
	if model.cloneAddonURL != "" {
		t.Fatalf("alt+e inserted %q", model.cloneAddonURL)
	}
}

func TestControlVInsertsClipboardIntoFocusedField(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.modal = modalCloneAddon
	model.formField = 1
	model.cloneAddonURL = "git@"

	_, cmd := model.updateKey(tea.KeyMsg{Type: tea.KeyCtrlV})
	if cmd == nil {
		t.Fatal("ctrl+v should read the clipboard")
	}
	// Do not execute the command: it would read the machine's real clipboard.
	model.Update(ClipboardContentMsg{Text: "github.com:acme/addon.git"})
	if model.cloneAddonURL != "git@github.com:acme/addon.git" {
		t.Fatalf("git url = %q", model.cloneAddonURL)
	}
}

func TestControlVWithoutTextFieldDoesNothing(t *testing.T) {
	model := NewModel(context.Background(), nil)

	_, cmd := model.updateKey(tea.KeyMsg{Type: tea.KeyCtrlV})
	if cmd != nil {
		t.Fatal("ctrl+v without a text field should not read the clipboard")
	}
}

func TestClipboardFailureShowsNotice(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.modal = modalCloneAddon
	model.formField = 1

	model.applyClipboardPaste(ClipboardContentMsg{Err: errors.New("no clipboard tool found")})
	if model.notice == "" {
		t.Fatal("clipboard failure should surface a notice")
	}
	if model.cloneAddonURL != "" {
		t.Fatalf("git url = %q, want untouched", model.cloneAddonURL)
	}
}

func TestSanitizePasteDropsControlCharacters(t *testing.T) {
	if got := sanitizePaste(" https://x/y\r\n\t"); got != " https://x/y" {
		t.Fatalf("sanitizePaste = %q", got)
	}
}
