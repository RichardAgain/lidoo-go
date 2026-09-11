package odoo

import (
	"bytes"
	"io"
	"regexp"
	"strings"
	"sync"
)

var knownOutputNoise = []string{
	"Warn: Can't find .pfb for face 'Courier'",
	"<string>:38: (ERROR/3) Unexpected indentation.",
	"<string>:43: (WARNING/2) Block quote ends without a blank line; unexpected unindent.",
}

var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*[[:alpha:]]`)

type cleanOutputWriter struct {
	destination io.Writer
	pending     []byte
	mu          sync.Mutex
}

func newCleanOutputWriter(destination io.Writer) *cleanOutputWriter {
	return &cleanOutputWriter{destination: destination}
}

func (w *cleanOutputWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.pending = append(w.pending, p...)
	for {
		index := bytes.IndexByte(w.pending, '\n')
		if index < 0 {
			break
		}
		if err := w.writeLine(w.pending[:index+1]); err != nil {
			return 0, err
		}
		w.pending = w.pending[index+1:]
	}
	return len(p), nil
}

func (w *cleanOutputWriter) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if len(w.pending) == 0 {
		return nil
	}
	err := w.writeLine(w.pending)
	w.pending = nil
	return err
}

func (w *cleanOutputWriter) writeLine(line []byte) error {
	clean := ansiEscape.ReplaceAll(line, nil)
	trimmed := strings.TrimRight(string(clean), "\r\n")
	for _, noise := range knownOutputNoise {
		if trimmed == noise {
			return nil
		}
	}
	if strings.Contains(trimmed, " INFO ") && strings.Contains(trimmed, " click_odoo_contrib.") {
		return nil
	}
	_, err := w.destination.Write(clean)
	return err
}
