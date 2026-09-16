package odoo

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"lidoo/internal/docker"
)

const execScript = `set -eu
export PGHOST="${HOST:-lidoo-postgres}"
export PGPORT="${PORT:-5432}"
export PGUSER="${POSTGRES_USER:?POSTGRES_USER must be set in the profile environment}"
export PGPASSWORD="${POSTGRES_PASSWORD:?POSTGRES_PASSWORD must be set in the profile environment}"
if ! command -v "$1" >/dev/null 2>&1; then
  echo "profile image is missing click-odoo-contrib (required command: $1); rebuild and recreate the profile" >&2
  exit 127
fi
exec "$@"
`

func run(container string, command ...string) error {
	return runWithCommandOptions(container, docker.CommandOptions{Stdout: os.Stdout, Stderr: os.Stderr}, command...)
}

func runWithOutput(container string, stdout, stderr io.Writer, command ...string) error {
	return runWithCommandOptions(container, docker.CommandOptions{Stdout: stdout, Stderr: stderr}, command...)
}

func runWithCommandOptions(container string, options docker.CommandOptions, command ...string) error {
	if len(command) == 0 {
		return errors.New("cannot execute an empty command")
	}

	args := make([]string, 0, len(command)+4)
	args = append(args, "sh", "-c", execScript, "lidoo")
	args = append(args, command...)

	if options.Stdout == nil {
		options.Stdout = os.Stdout
	}
	if options.Stderr == nil {
		options.Stderr = os.Stderr
	}
	stdoutWriter := newCleanOutputWriter(options.Stdout)
	stderrWriter := newCleanOutputWriter(options.Stderr)
	options.Stdout = stdoutWriter
	options.Stderr = stderrWriter
	err := docker.ExecWithOutputOptions(container, options, args...)
	if flushErr := stdoutWriter.Flush(); err == nil {
		err = flushErr
	}
	if flushErr := stderrWriter.Flush(); err == nil {
		err = flushErr
	}
	return err
}

func runCapture(container string, command ...string) ([]byte, error) {
	var output, diagnostic bytes.Buffer
	if err := runWithOutput(container, &output, &diagnostic, command...); err != nil {
		detail := strings.TrimSpace(diagnostic.String())
		if detail != "" {
			return nil, fmt.Errorf("%w: %s", err, detail)
		}
		return nil, err
	}
	return output.Bytes(), nil
}

func runInteractive(container string, command ...string) error {
	args := make([]string, 0, len(command)+4)
	args = append(args, "sh", "-c", execScript, "lidoo")
	args = append(args, command...)
	return docker.ExecInteractive(container, args...)
}
