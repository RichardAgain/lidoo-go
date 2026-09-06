package docker

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"regexp"
)

const (
	databaseOperationInit   = "init"
	databaseOperationUpdate = "update"
	databaseOperationDrop   = "drop"
)

var databaseNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$`)

const databaseEnvironmentScript = `set -eu
export PGHOST="${HOST:-lidoo-postgres}"
export PGPORT="${PORT:-5432}"
export PGUSER="${POSTGRES_USER:?POSTGRES_USER must be set in the profile environment}"
export PGPASSWORD="${POSTGRES_PASSWORD:?POSTGRES_PASSWORD must be set in the profile environment}"
`

func databaseToolCheckScript(binary string) string {
	return fmt.Sprintf(`if ! command -v %s >/dev/null 2>&1; then
  echo "profile image is missing click-odoo-contrib (required command: %s); rebuild and recreate the profile" >&2
  exit 127
fi
`, binary, binary)
}

func Init(args []string) error {
	flags := flag.NewFlagSet("init", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	name := flags.String("name", "", "profile name")
	database := flags.String("database", "", "database name")
	modules := flags.String("modules", "base", "comma-separated modules to install")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("init does not accept positional arguments")
	}
	if err := validateDatabaseOperationInputs(*name, *database); err != nil {
		return err
	}
	if *modules == "" {
		return errors.New("init requires a non-empty --modules value")
	}

	container, err := requireRunningProfile(*name)
	if err != nil {
		return err
	}
	fmt.Printf("initializing database %q in profile %q (modules: %s)\n", *database, *name, *modules)
	if err := dockerCleanDatabaseOutput(databaseCommand(container, databaseOperationInit, *database, *modules, false)...); err != nil {
		return fmt.Errorf("initialize database %q: %w", *database, err)
	}
	fmt.Printf("database %q initialized\n", *database)
	return nil
}

func Update(args []string) error {
	flags := flag.NewFlagSet("update", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	name := flags.String("name", "", "profile name")
	database := flags.String("database", "", "database name")
	updateAll := flags.Bool("update-all", false, "force a complete module update")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("update does not accept positional arguments")
	}
	if err := validateDatabaseOperationInputs(*name, *database); err != nil {
		return err
	}

	container, err := requireRunningProfile(*name)
	if err != nil {
		return err
	}
	fmt.Printf("updating database %q in profile %q\n", *database, *name)
	if err := dockerCleanDatabaseOutput(databaseCommand(container, databaseOperationUpdate, *database, "", *updateAll)...); err != nil {
		return fmt.Errorf("update database %q: %w", *database, err)
	}
	fmt.Printf("database %q updated\n", *database)
	return nil
}

func Drop(args []string) error {
	flags := flag.NewFlagSet("drop", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	name := flags.String("name", "", "profile name")
	database := flags.String("database", "", "database name")
	var yes bool
	flags.BoolVar(&yes, "y", false, "confirm dropping the database")
	flags.BoolVar(&yes, "yes", false, "confirm dropping the database")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("drop does not accept positional arguments")
	}
	if *name == "" {
		return errors.New("database operation requires --name")
	}
	if err := validateProfileName(*name); err != nil {
		return err
	}
	if err := validateDropDatabase(*database); err != nil {
		return err
	}
	if !yes {
		return errors.New("drop requires --yes")
	}

	container, err := requireRunningProfile(*name)
	if err != nil {
		return err
	}
	fmt.Printf("dropping database %q from profile %q\n", *database, *name)
	if err := dockerCleanDatabaseOutput(databaseCommand(container, databaseOperationDrop, *database, "", false)...); err != nil {
		return fmt.Errorf("drop database %q: %w", *database, err)
	}
	fmt.Printf("database %q dropped\n", *database)
	return nil
}

func validateDatabaseOperationInputs(profile, database string) error {
	if profile == "" {
		return errors.New("database operation requires --name")
	}
	if err := validateProfileName(profile); err != nil {
		return err
	}
	return validateDatabaseName(database)
}

func validateDatabaseName(name string) error {
	if !databaseNamePattern.MatchString(name) {
		return fmt.Errorf("invalid database name %q: use 1-63 letters, numbers, dots, underscores, or hyphens", name)
	}
	return nil
}

func validateDropDatabase(name string) error {
	if err := validateDatabaseName(name); err != nil {
		return err
	}
	switch name {
	case "postgres", "template0", "template1":
		return fmt.Errorf("refusing to drop protected database %q", name)
	default:
		return nil
	}
}

func requireRunningProfile(name string) (string, error) {
	exists, err := findContainerByName(name)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", fmt.Errorf("no profile with name %q", name)
	}
	running, err := containerIsRunning(name)
	if err != nil {
		return "", err
	}
	if !running {
		return "", fmt.Errorf("profile %q is not running", name)
	}
	return "lidoo-" + name, nil
}

func databaseCommand(container, operation, database, modules string, updateAll bool) []string {
	var script string
	switch operation {
	case databaseOperationInit:
		script = databaseEnvironmentScript + databaseToolCheckScript("click-odoo-initdb") + `exec click-odoo-initdb --log-level=error \
  --new-database "$1" --modules "$2"
`
	case databaseOperationUpdate:
		script = databaseEnvironmentScript + databaseToolCheckScript("click-odoo-update") + `exec click-odoo-update --log-level=error \
  --database "$1"`
		if updateAll {
			script += " --update-all"
		}
		script += "\n"
	case databaseOperationDrop:
		script = databaseEnvironmentScript + databaseToolCheckScript("click-odoo-dropdb") + `exec click-odoo-dropdb --log-level=error \
  "$1"
`
	default:
		return nil
	}

	command := []string{"exec", container, "sh", "-c", script, "lidoo", database}
	if operation == databaseOperationInit {
		command = append(command, modules)
	}
	return command
}
