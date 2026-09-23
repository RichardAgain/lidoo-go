package tui

import (
	"context"
	"strings"
	"testing"

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
