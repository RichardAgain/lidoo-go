package odoo

import (
	"fmt"

	"lidoo/internal/docker"
	"lidoo/internal/files"
)

func Update(name, database string, updateAll bool, state files.State, options ...OperationOptions) (result OperationResult, err error) {
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

	commandOptions := stdout.commandOptions(stderr, operationOptions.Context)
	container, err := docker.RequireRunningProfileWithOptions(name, commandOptions)
	if err != nil {
		return result, err
	}
	fmt.Fprintf(stdout, "updating database %q in profile %q\n", database, name)
	args := []string{
		"click-odoo-update",
		"--log-level=error",
		"--database", database,
	}
	if updateAll {
		args = append(args, "--update-all")
	}
	if err := runWithCommandOptions(container, commandOptions, args...); err != nil {
		return result, fmt.Errorf("update database %q: %w", database, err)
	}
	fmt.Fprintf(stdout, "database %q updated\n", database)
	return result, nil
}
