package tui

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Zinc palette on a black background. Only white is an accent; green, amber
// and red are reserved for state.
const (
	colorBg     = lipgloss.Color("#000000")
	colorWhite  = lipgloss.Color("#fafafa")
	colorText   = lipgloss.Color("#e4e4e7")
	colorSecond = lipgloss.Color("#a1a1aa")
	colorMuted  = lipgloss.Color("#71717a")
	colorDim    = lipgloss.Color("#52525b")
	colorRule   = lipgloss.Color("#27272a")
	colorOK     = lipgloss.Color("#34d399")
	colorWarn   = lipgloss.Color("#fbbf24")
	colorError  = lipgloss.Color("#f87171")
)

var (
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(colorWhite)
	activeStyle  = lipgloss.NewStyle().Bold(true).Foreground(colorWhite)
	textStyle    = lipgloss.NewStyle().Foreground(colorText)
	keyStyle     = lipgloss.NewStyle().Bold(true).Foreground(colorText)
	secondStyle  = lipgloss.NewStyle().Foreground(colorSecond)
	errorStyle   = lipgloss.NewStyle().Foreground(colorError)
	mutedStyle   = lipgloss.NewStyle().Foreground(colorMuted)
	dimStyle     = lipgloss.NewStyle().Foreground(colorDim)
	ruleStyle    = lipgloss.NewStyle().Foreground(colorRule)
	runningStyle = lipgloss.NewStyle().Foreground(colorOK)
	warningStyle = lipgloss.NewStyle().Foreground(colorWarn)

	borderStyle          = colorRule
	activeBorderStyle    = colorDim
	inspectedBorderStyle = colorDim
)

// frameBase is the escape sequence that sets the default look of every cell:
// black background and body-text foreground.
const frameBase = "\x1b[48;2;0;0;0m\x1b[38;2;228;228;231m"

var sgrReset = regexp.MustCompile(`\x1b\[0?m`)

// paintFrame makes every cell of the frame black. Nested lipgloss renders end
// with a reset that falls back to the terminal's own colors, so the base look
// is re-asserted after every reset and each line is padded to the full width.
func paintFrame(view string, width, height int) string {
	lines := strings.Split(view, "\n")
	if height > 0 {
		for len(lines) < height {
			lines = append(lines, "")
		}
		if len(lines) > height {
			lines = lines[:height]
		}
	}
	for index, line := range lines {
		if gap := width - ansi.StringWidth(line); gap > 0 {
			line += strings.Repeat(" ", gap)
		}
		line = sgrReset.ReplaceAllString(line, "\x1b[0m"+frameBase)
		lines[index] = frameBase + line + "\x1b[0m"
	}
	return strings.Join(lines, "\n")
}

// View renders the dashboard on the enforced black surface.
func (m *Model) View() string {
	return paintFrame(m.view(), m.width, m.height)
}
