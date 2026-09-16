package odoo

import (
	"errors"
	"fmt"

	"lidoo/internal/docker"
	"lidoo/internal/files"
)

func Init(name, database, modules string, state files.State, options ...OperationOptions) (result OperationResult, err error) {
	stdout, stderr, operationOptions := newOperationStreams(options)
	result.ProfileName = name
	defer func() {
		result.Output = stdout.String()
		result.ErrorOutput = stderr.String()
	}()

	logicalDatabase := database
	database, err = resolveDatabaseName(state, name, database)
	if err != nil {
		return result, err
	}
	result.LogicalDatabase = logicalDatabase
	result.PhysicalDatabase = database
	if modules == "" {
		return result, errors.New("init requires a non-empty modules value")
	}

	commandOptions := stdout.commandOptions(stderr, operationOptions.Context)
	container, err := docker.RequireRunningProfileWithOptions(name, commandOptions)
	if err != nil {
		return result, err
	}
	fmt.Fprintf(stdout, "initializing database %q in profile %q (modules: %s)\n", database, name, modules)
	args := []string{
		"click-odoo-initdb",
		"--log-level=error",
		"--new-database", database,
		"--modules", modules,
	}
	if err := runWithCommandOptions(container, commandOptions, args...); err != nil {
		return result, fmt.Errorf("initialize database %q: %w", database, err)
	}
	fmt.Fprintf(stdout, "database %q initialized\n", database)
	return result, nil
}
