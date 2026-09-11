package odoo

import (
	"errors"
	"os"

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
	if len(command) == 0 {
		return errors.New("cannot execute an empty command")
	}

	args := make([]string, 0, len(command)+4)
	args = append(args, "sh", "-c", execScript, "lidoo")
	args = append(args, command...)

	writer := newCleanOutputWriter(os.Stdout)
	err := docker.ExecWithOutput(container, writer, writer, args...)
	if flushErr := writer.Flush(); err == nil {
		err = flushErr
	}
	return err
}
