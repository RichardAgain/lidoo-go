package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"lidoo/internal/docker"
)

func TestProfileFocusShowsLogsWithoutLogTab(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.loading = false
	model.profiles = []docker.ProfileSummary{{Name: "testing", State: "running"}}
	model.profileLogs["testing"] = &profileLogBuffer{output: "profile log output"}
	model.logPhase = phaseReady

	view := ansi.Strip(model.rightView(60))
	if !strings.Contains(view, "profile log output") {
		t.Fatalf("profile focus should show logs, got %q", view)
	}
	if strings.Contains(view, "Profile details") {
		t.Fatalf("profile focus should not show profile details, got %q", view)
	}
	if strings.Contains(view, "Log") {
		t.Fatalf("log tab should be removed, got %q", view)
	}
}

func TestStoppedProfileShowsLogMessage(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.profiles = []docker.ProfileSummary{{Name: "testing", State: "exited"}}
	model.profileLogs["testing"] = &profileLogBuffer{output: "stale log output"}
	model.logPhase = phaseReady

	const want = "start the container to see logs"
	if got := ansi.Strip(model.logContent()); got != want {
		t.Fatalf("log content = %q, want %q", got, want)
	}
	if cmd := model.beginLogLoad(); cmd != nil {
		t.Fatal("stopped profile should not start a log command")
	}
	if model.logPhase != phaseUnavailable {
		t.Fatalf("log phase = %v, want unavailable", model.logPhase)
	}
}

func TestCreateProfileVersionSelection(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.modal = modalCreateProfile
	model.formField = 1
	model.createProfileVersion = selectableOdooVersions[0]

	model.updateModalKey(tea.KeyMsg{Type: tea.KeyDown})
	if model.createProfileVersion != "18" {
		t.Fatalf("version after down = %q, want 18", model.createProfileVersion)
	}
	model.updateModalKey(tea.KeyMsg{Type: tea.KeyUp})
	if model.createProfileVersion != "17" {
		t.Fatalf("version after up = %q, want 17", model.createProfileVersion)
	}
	model.updateModalKey(tea.KeyMsg{Type: tea.KeyUp})
	if model.createProfileVersion != "19" {
		t.Fatalf("version should wrap to 19, got %q", model.createProfileVersion)
	}
}

func TestVersionSelectorRendersStaticChoices(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.modal = modalCreateProfile
	model.formField = 1
	model.createProfileVersion = "18"

	view := ansi.Strip(model.modalView())
	for _, version := range []string{"17", "18", "19"} {
		if !strings.Contains(view, version) {
			t.Fatalf("version %s missing from selector: %q", version, view)
		}
	}
	if !strings.Contains(view, "> 18") {
		t.Fatalf("selected version missing from selector: %q", view)
	}
}
