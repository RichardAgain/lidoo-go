package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestPaintFrameCoversEveryCell(t *testing.T) {
	view := "plain\n\x1b[38;2;1;2;3mcolored\x1b[0m tail\x1b[m more"
	painted := paintFrame(view, 20, 4)
	lines := strings.Split(painted, "\n")
	if len(lines) != 4 {
		t.Fatalf("lines = %d, want 4", len(lines))
	}
	for _, line := range lines {
		if !strings.HasPrefix(line, frameBase) {
			t.Fatalf("line %q does not start with the base look", line)
		}
		if width := ansi.StringWidth(line); width != 20 {
			t.Fatalf("line width = %d, want 20", width)
		}
		// Every reset must be followed by the base look again.
		for _, reset := range []string{"\x1b[0m", "\x1b[m"} {
			rest := line
			for {
				index := strings.Index(rest, reset)
				if index < 0 {
					break
				}
				after := rest[index+len(reset):]
				if after != "" && !strings.HasPrefix(after, frameBase) {
					t.Fatalf("reset in %q is not followed by the base look", line)
				}
				rest = after
			}
		}
	}
}

func TestViewIsPaintedAtFullSize(t *testing.T) {
	model := NewModel(context.Background(), nil)
	model.width, model.height = 100, 30
	model.loading = false
	lines := strings.Split(model.View(), "\n")
	if len(lines) != 30 {
		t.Fatalf("lines = %d, want 30", len(lines))
	}
	for _, line := range lines {
		if ansi.StringWidth(line) != 100 || !strings.HasPrefix(line, frameBase) {
			t.Fatalf("line is not painted to full width: %q", line)
		}
	}
}
