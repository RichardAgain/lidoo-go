package docker

import (
	"bytes"
	"testing"
)

func TestCleanDatabaseOutputRemovesOnlyKnownUpstreamNoise(t *testing.T) {
	input := "" +
		"Warn: Can't find .pfb for face 'Courier'\n" +
		"<string>:38: (ERROR/3) Unexpected indentation.\n" +
		"<string>:43: (WARNING/2) Block quote ends without a blank line; unexpected unindent.\n" +
		"\x1b[32m2026-09-03 12:48:10 INFO Database updated\x1b[0m\n" +
		"2026-09-03 12:48:11 INFO testing_db click_odoo_contrib.update: No module needs updating, update is not performed.\n" +
		"ERROR real database failure\n"

	var output bytes.Buffer
	writer := newCleanDatabaseOutputWriter(&output)
	if _, err := writer.Write([]byte(input)); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := writer.Flush(); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	want := "2026-09-03 12:48:10 INFO Database updated\nERROR real database failure\n"
	if output.String() != want {
		t.Fatalf("cleaned output = %q, want %q", output.String(), want)
	}
}
