package tui

import (
	"strings"
	"testing"
)

func TestParseOdooLogLineCompactsWerkzeug(t *testing.T) {
	line := `2026-07-27 20:22:21,925 227050 INFO ? werkzeug: 127.0.0.1 - - [27/Jul/2026 20:22:21] "GET /web/login HTTP/1.1" 200 - 0 0.003 0.026`
	entry := parseLogLine(logSourceContainer, line)

	if entry.Level != logLevelInfo {
		t.Fatalf("level = %v, want info", entry.Level)
	}
	if entry.Time != "20:22:21" {
		t.Fatalf("time = %q, want 20:22:21", entry.Time)
	}
	want := "werkzeug: GET /web/login → 200  0q 29ms"
	if entry.Text != want {
		t.Fatalf("text = %q, want %q", entry.Text, want)
	}
}

func TestParseOdooLogLineKeepsDatabaseAndLevel(t *testing.T) {
	line := `2026-01-02 03:04:05,678 9 INFO mydb odoo.modules.loading: Modules loaded. 12 0.500 0.100`
	entry := parseLogLine(logSourceContainer, line)

	if entry.Level != logLevelInfo {
		t.Fatalf("level = %v, want info", entry.Level)
	}
	if entry.Database != "mydb" {
		t.Fatalf("database = %q, want mydb", entry.Database)
	}
	want := "modules.loading: Modules loaded.  12q 600ms"
	if entry.Text != want {
		t.Fatalf("text = %q, want %q", entry.Text, want)
	}
}

func TestParseOdooLogLineMapsErrorLevel(t *testing.T) {
	line := `2026-01-02 03:04:05,678 9 ERROR demodb odoo.sql_db: bad query`
	entry := parseLogLine(logSourceContainer, line)
	if entry.Level != logLevelError {
		t.Fatalf("level = %v, want error", entry.Level)
	}
}

func TestParseRawLinesSurvive(t *testing.T) {
	line := "Traceback (most recent call last):"
	entry := parseLogLine(logSourceContainer, line)
	if entry.Level != logLevelRaw || entry.Text != line {
		t.Fatalf("entry = %+v", entry)
	}

	taskEntry := parseLogLine(logSourceTask, "click-odoo-update: done")
	if taskEntry.Source != logSourceTask || taskEntry.Level != logLevelRaw || taskEntry.Text != "click-odoo-update: done" {
		t.Fatalf("task entry = %+v", taskEntry)
	}
}

func TestLevelFilterKeepsRawLines(t *testing.T) {
	if !logLevelFilterError.allows(logLevelRaw) {
		t.Fatal("error filter should keep raw lines")
	}
	if logLevelFilterError.allows(logLevelInfo) {
		t.Fatal("error filter should drop info lines")
	}
	if logLevelFilterInfo.allows(logLevelDebug) {
		t.Fatal("info filter should drop debug lines")
	}
	if !logLevelFilterWarning.allows(logLevelError) {
		t.Fatal("warn filter should keep error lines")
	}
}

func TestSourceFilter(t *testing.T) {
	if !logSourceOnlyContainer.allows(logSourceContainer) || logSourceOnlyContainer.allows(logSourceTask) {
		t.Fatal("container filter is wrong")
	}
	if !logSourceOnlyTask.allows(logSourceTask) || logSourceOnlyTask.allows(logSourceContainer) {
		t.Fatal("task filter is wrong")
	}
}

func TestContainerLogEntriesDropsTasks(t *testing.T) {
	entries := buildLogEntries("hello container\n", "task line\n")
	container := containerLogEntries(entries)
	if len(container) != 1 {
		t.Fatalf("container entries = %d, want 1", len(container))
	}
	if !strings.Contains(renderLogEntries(container, false), "hello container") {
		t.Fatalf("rendered = %q", renderLogEntries(container, false))
	}
	if strings.Contains(renderLogEntries(container, false), "task line") {
		t.Fatalf("container view leaked task output")
	}
}
