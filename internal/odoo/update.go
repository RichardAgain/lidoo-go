package odoo

import (
	"fmt"

	"lidoo/internal/docker"
	"lidoo/internal/files"
)

func Update(name, database string, updateAll bool, state files.State) error {
	database, err := resolveDatabaseName(state, name, database)
	if err != nil {
		return err
	}

	container, err := docker.RequireRunningProfile(name)
	if err != nil {
		return err
	}
	fmt.Printf("updating database %q in profile %q\n", database, name)
	args := []string{
		"click-odoo-update",
		"--log-level=error",
		"--database", database,
	}
	if updateAll {
		args = append(args, "--update-all")
	}
	if err := run(container, args...); err != nil {
		return fmt.Errorf("update database %q: %w", database, err)
	}
	fmt.Printf("database %q updated\n", database)
	return nil
}
