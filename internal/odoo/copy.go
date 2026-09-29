package odoo

import (
	"errors"
	"fmt"

	"lidoo/internal/docker"
	"lidoo/internal/files"
)

func Copy(name, source, destination string, forceDisconnect, unlessDestExists, ifSourceExists bool, filestoreCopyMode string, state files.State, options ...OperationOptions) (result OperationResult, err error) {
	stdout, stderr, operationOptions := newOperationStreams(options)
	result.ProfileName = name
	defer func() {
		result.Output = stdout.String()
		result.ErrorOutput = stderr.String()
	}()

	if filestoreCopyMode != "default" && filestoreCopyMode != "rsync" && filestoreCopyMode != "hardlink" {
		return result, fmt.Errorf("invalid filestore copy mode %q: use default, rsync, or hardlink", filestoreCopyMode)
	}
	if err := validateDatabaseOperationInputs(name, source); err != nil {
		return result, err
	}
	if err := validateDatabaseName(destination); err != nil {
		return result, err
	}
	if err := validateDropDatabase(destination); err != nil {
		return result, err
	}

	logicalDestination := destination
	found, lookupErr := findDatabase(state, name, source)
	if lookupErr == nil {
		source = found.Physical
	} else if ifSourceExists && errors.Is(lookupErr, ErrDatabaseNotFound) {
		source, err = resolveDatabaseName(state, name, source)
	} else {
		return result, lookupErr
	}
	if err != nil {
		return result, err
	}
	destination, err = resolveDatabaseName(state, name, destination)
	if err != nil {
		return result, err
	}
	if source == destination {
		return result, errors.New("source and destination databases must differ")
	}
	result.LogicalDatabase = logicalDestination
	result.PhysicalDatabase = destination

	commandOptions := stdout.commandOptions(stderr, operationOptions.Context)
	container, err := docker.RequireRunningProfileWithOptions(name, commandOptions)
	if err != nil {
		return result, err
	}
	fmt.Fprintf(stdout, "copying database %q to %q in profile %q\n", source, destination, name)
	args := []string{"click-odoo-copydb", "--log-level=error"}
	if forceDisconnect {
		args = append(args, "--force-disconnect")
	}
	if unlessDestExists {
		args = append(args, "--unless-dest-exists")
	}
	if ifSourceExists {
		args = append(args, "--if-source-exists")
	}
	args = append(args, "--filestore-copy-mode", filestoreCopyMode, source, destination)
	if err := runWithCommandOptions(container, commandOptions, args...); err != nil {
		return result, fmt.Errorf("copy database %q to %q: %w", source, destination, err)
	}
	fmt.Fprintf(stdout, "copy database %q to %q completed\n", source, destination)
	return result, nil
}
