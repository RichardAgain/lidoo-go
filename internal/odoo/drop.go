package odoo

import (
	"errors"
	"fmt"

	"lidoo/internal/docker"
	"lidoo/internal/files"
)

func Drop(name, database string, yes bool, state files.State) error {
	if err := validateDatabaseOperationInputs(name, database); err != nil {
		return err
	}
	if err := validateDropDatabase(database); err != nil {
		return err
	}
	if !yes {
		return errors.New("drop requires --yes")
	}

	database, err := resolveDatabaseName(state, name, database)
	if err != nil {
		return err
	}

	container, err := docker.RequireRunningProfile(name)
	if err != nil {
		return err
	}
	fmt.Printf("dropping database %q from profile %q\n", database, name)
	args := []string{
		"click-odoo-dropdb",
		"--log-level=error",
		database,
	}
	if err := run(container, args...); err != nil {
		return fmt.Errorf("drop database %q: %w", database, err)
	}
	fmt.Printf("database %q dropped\n", database)
	return nil
}
