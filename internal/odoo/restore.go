package odoo

import (
	"fmt"
	"os"

	"lidoo/internal/docker"
	"lidoo/internal/files"
)

func Restore(name, database, source string, copyDatabase, force, neutralize bool, jobs int, state files.State, options ...OperationOptions) (result OperationResult, err error) {
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
	if jobs < 1 {
		return result, fmt.Errorf("restore jobs must be at least 1")
	}
	source, err = absolutePath(source, "restore source")
	if err != nil {
		return result, err
	}
	result.Destination = source
	if _, err := os.Stat(source); err != nil {
		return result, fmt.Errorf("restore source %q: %w", source, err)
	}

	commandOptions := stdout.commandOptions(stderr, operationOptions.Context)
	container, err := docker.RequireRunningProfileWithOptions(name, commandOptions)
	if err != nil {
		return result, err
	}

	containerSource := temporaryContainerPath("restore")
	defer removeContainerPathWithOptions(container, containerSource, commandOptions)

	fmt.Fprintf(stdout, "restoring database %q in profile %q from %q\n", database, name, source)
	if err := docker.CopyToWithOptions(container, source, containerSource, commandOptions); err != nil {
		return result, fmt.Errorf("copy restore source to profile: %w", err)
	}
	if err := docker.ExecAsUserWithOptions(container, "root", commandOptions, "chown", "-R", "odoo", containerSource); err != nil {
		return result, fmt.Errorf("make restore source readable: %w", err)
	}

	args := []string{
		"click-odoo-restoredb",
		"--log-level=error",
		"--jobs", fmt.Sprintf("%d", jobs),
	}
	if copyDatabase {
		args = append(args, "--copy")
	} else {
		args = append(args, "--move")
	}
	if force {
		args = append(args, "--force")
	}
	if neutralize {
		args = append(args, "--neutralize")
	}
	args = append(args, database, containerSource)

	if err := runWithCommandOptions(container, commandOptions, args...); err != nil {
		return result, fmt.Errorf("restore database %q: %w", database, err)
	}
	fmt.Fprintf(stdout, "database %q restored\n", database)
	return result, nil
}
