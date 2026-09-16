package odoo

import (
	"errors"
	"fmt"

	"lidoo/internal/docker"
	"lidoo/internal/files"
)

func Drop(name, database string, yes bool, state files.State, options ...OperationOptions) (result OperationResult, err error) {
	stdout, stderr, operationOptions := newOperationStreams(options)
	result.ProfileName = name
	defer func() {
		result.Output = stdout.String()
		result.ErrorOutput = stderr.String()
	}()

	if err := validateDatabaseOperationInputs(name, database); err != nil {
		return result, err
	}
	if err := validateDropDatabase(database); err != nil {
		return result, err
	}
	if !yes {
		return result, errors.New("drop requires --yes")
	}

	logicalDatabase := database
	database, err = resolveExistingDatabaseName(state, name, database)
	if err != nil {
		return result, err
	}
	result.LogicalDatabase = logicalDatabase
	result.PhysicalDatabase = database

	commandOptions := stdout.commandOptions(stderr, operationOptions.Context)
	container, err := docker.RequireRunningProfileWithOptions(name, commandOptions)
	if err != nil {
		return result, err
	}
	fmt.Fprintf(stdout, "dropping database %q from profile %q\n", database, name)
	args := []string{
		"click-odoo-dropdb",
		"--log-level=error",
		database,
	}
	if err := runWithCommandOptions(container, commandOptions, args...); err != nil {
		return result, fmt.Errorf("drop database %q: %w", database, err)
	}
	fmt.Fprintf(stdout, "database %q dropped\n", database)
	return result, nil
}
