package tui

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// logSource distinguishes container output from task/operation output so the
// two streams can be read separately.
type logSource uint8

const (
	logSourceContainer logSource = iota
	logSourceTask
)

// logLevel is the severity parsed from an Odoo log prefix. Raw lines have no
// level and are shown verbatim.
type logLevel uint8

const (
	logLevelRaw logLevel = iota
	logLevelDebug
	logLevelInfo
	logLevelWarning
	logLevelError
)

type logEntry struct {
	Source   logSource
	Level    logLevel
	Time     string
	Database string
	Text     string
}

type logSourceFilter uint8

const (
	logSourceAll logSourceFilter = iota
	logSourceOnlyContainer
	logSourceOnlyTask
)

func (filter logSourceFilter) allows(source logSource) bool {
	switch filter {
	case logSourceOnlyContainer:
		return source == logSourceContainer
	case logSourceOnlyTask:
		return source == logSourceTask
	default:
		return true
	}
}

type logLevelFilter uint8

const (
	logLevelFilterAll logLevelFilter = iota
	logLevelFilterInfo
	logLevelFilterWarning
	logLevelFilterError
)

// allows keeps raw (non-Odoo) lines regardless of the minimum level so
// tracebacks and Docker diagnostics are never hidden.
func (filter logLevelFilter) allows(level logLevel) bool {
	if level == logLevelRaw || filter == logLevelFilterAll {
		return true
	}
	var minimum logLevel
	switch filter {
	case logLevelFilterInfo:
		minimum = logLevelInfo
	case logLevelFilterWarning:
		minimum = logLevelWarning
	case logLevelFilterError:
		minimum = logLevelError
	default:
		return true
	}
	return level >= minimum
}

var (
	odooLogPrefix = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}) (\d{2}:\d{2}:\d{2}),\d{3} (\d+) ([A-Z]+) (\S+) ([\w.]+): (.*)$`)
	odooPerf      = regexp.MustCompile(` (\d+) (\d+\.\d+) (\d+\.\d+)$`)
	odooPerfEmpty = regexp.MustCompile(` - - -$`)
	odooAccess    = regexp.MustCompile(`^\S+ - - \[[^\]]*\] "([A-Z]+) (\S+) [^"]*" (\d{3}) (\S+)$`)
)

func buildLogEntries(container, task string) []logEntry {
	entries := make([]logEntry, 0, 64)
	entries = appendLogLines(entries, logSourceContainer, container)
	entries = appendLogLines(entries, logSourceTask, task)
	return entries
}

func appendLogLines(entries []logEntry, source logSource, text string) []logEntry {
	if text == "" {
		return entries
	}
	text = strings.TrimSuffix(text, "\n")
	for _, line := range strings.Split(text, "\n") {
		entries = append(entries, parseLogLine(source, strings.TrimSuffix(line, "\r")))
	}
	return entries
}

func parseLogLine(source logSource, line string) logEntry {
	if source != logSourceContainer {
		return logEntry{Source: source, Level: logLevelRaw, Text: line}
	}
	match := odooLogPrefix.FindStringSubmatch(line)
	if match == nil {
		return logEntry{Source: source, Level: logLevelRaw, Text: line}
	}

	logger := match[6]
	if strings.HasPrefix(logger, "odoo.") {
		logger = logger[len("odoo."):]
	}
	database := ""
	if value := match[5]; value != "" && value != "?" {
		database = value
	}
	return logEntry{
		Source:   source,
		Level:    odooLevel(match[4]),
		Time:     match[2],
		Database: database,
		Text:     logger + ": " + compactOdooMessage(match[7]),
	}
}

func odooLevel(value string) logLevel {
	switch value {
	case "DEBUG":
		return logLevelDebug
	case "INFO":
		return logLevelInfo
	case "WARNING":
		return logLevelWarning
	case "ERROR", "CRITICAL":
		return logLevelError
	default:
		return logLevelInfo
	}
}

// compactOdooMessage removes the noise Odoo adds to every message: the trailing
// perf_info counters become "Nq Xms" and werkzeug access lines collapse to
// "METHOD PATH → STATUS".
func compactOdooMessage(message string) string {
	perf := ""
	switch {
	case odooPerfEmpty.MatchString(message):
		message = odooPerfEmpty.ReplaceAllString(message, "")
	default:
		if match := odooPerf.FindStringSubmatch(message); match != nil {
			perf = fmt.Sprintf("%sq %dms", match[1], perfMilliseconds(match[2])+perfMilliseconds(match[3]))
			message = odooPerf.ReplaceAllString(message, "")
		}
	}
	message = strings.TrimRight(message, " ")
	if match := odooAccess.FindStringSubmatch(message); match != nil {
		message = match[1] + " " + match[2] + " → " + match[3]
	}
	if perf != "" {
		message += "  " + perf
	}
	return message
}

func perfMilliseconds(value string) int {
	var seconds float64
	if _, err := fmt.Sscanf(value, "%f", &seconds); err != nil {
		return 0
	}
	return int(seconds*1000 + 0.5)
}

func logLevelTag(level logLevel) string {
	switch level {
	case logLevelDebug:
		return "DEBUG"
	case logLevelInfo:
		return "INFO "
	case logLevelWarning:
		return "WARN "
	case logLevelError:
		return "ERROR"
	default:
		return "     "
	}
}

func logLevelStyle(level logLevel) lipgloss.Style {
	switch level {
	case logLevelDebug:
		return mutedStyle
	case logLevelWarning:
		return warningStyle
	case logLevelError:
		return errorStyle
	default:
		return lipgloss.NewStyle()
	}
}

func renderLogEntry(entry logEntry, showSource bool) string {
	prefix := ""
	if showSource {
		if entry.Source == logSourceTask {
			prefix = mutedStyle.Render("task") + " "
		} else {
			prefix = mutedStyle.Render("cont") + " "
		}
	}
	if entry.Level == logLevelRaw {
		return prefix + entry.Text
	}
	tag := logLevelStyle(entry.Level).Render(logLevelTag(entry.Level))
	body := entry.Text
	if entry.Database != "" {
		body = mutedStyle.Render("["+entry.Database+"]") + " " + body
	}
	if entry.Time == "" {
		return prefix + tag + " " + body
	}
	return prefix + tag + " " + mutedStyle.Render(entry.Time) + " " + body
}

func renderLogEntries(entries []logEntry, showSource bool) string {
	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		lines = append(lines, renderLogEntry(entry, showSource))
	}
	return strings.Join(lines, "\n")
}

// containerLogEntries drops task output so the inline log panel shows only the
// container stream. Task output lives in the "Latest task" section and in the
// viewer's tasks tab.
func containerLogEntries(entries []logEntry) []logEntry {
	container := make([]logEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Source == logSourceContainer {
			container = append(container, entry)
		}
	}
	return container
}

// filterLogEntriesByDatabase keeps only lines bound to one database. An empty
// database means "all", and lines without a bound database are dropped when a
// filter is active.
func filterLogEntriesByDatabase(entries []logEntry, database string) []logEntry {
	if database == "" {
		return entries
	}
	filtered := make([]logEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Database == database {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}
