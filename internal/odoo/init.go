package odoo

import (
	"errors"
	"fmt"

	"lidoo/internal/docker"
	"lidoo/internal/files"
)

func Init(name, database, modules string, state files.State) error {
	database, err := resolveDatabaseName(state, name, database)
	if err != nil {
		return err
	}
	if modules == "" {
		return errors.New("init requires a non-empty modules value")
	}

	container, err := docker.RequireRunningProfile(name)
	if err != nil {
		return err
	}
	fmt.Printf("initializing database %q in profile %q (modules: %s)\n", database, name, modules)
	args := []string{
		"click-odoo-initdb",
		"--log-level=error",
		"--new-database", database,
		"--modules", modules,
	}
	if err := run(container, args...); err != nil {
		return fmt.Errorf("initialize database %q: %w", database, err)
	}
	fmt.Printf("database %q initialized\n", database)
	return nil
}
