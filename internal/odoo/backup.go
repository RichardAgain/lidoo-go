package odoo

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"lidoo/internal/docker"
	"lidoo/internal/files"
)

var backupFormats = map[string]bool{
	"zip":    true,
	"dump":   true,
	"folder": true,
}

func Backup(name, database, destination, format string, force, ifExists, filestore bool, state files.State, options ...OperationOptions) (result OperationResult, err error) {
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
	if err := validateBackupFormat(format); err != nil {
		return result, err
	}
	if destination == "" {
		if format != "zip" {
			return result, fmt.Errorf("a backup destination is required when using format %q", format)
		}
		destination = defaultBackupDestination(logicalDatabase)
	}
	destination, err = absolutePath(destination, "backup destination")
	if err != nil {
		return result, err
	}
	result.Destination = destination
	if err := validateBackupDestination(destination); err != nil {
		return result, err
	}
	if !force && !ifExists {
		if _, err := os.Lstat(destination); err == nil {
			return result, fmt.Errorf("destination already exists: %s", destination)
		} else if !os.IsNotExist(err) {
			return result, fmt.Errorf("inspect destination: %w", err)
		}
	}

	commandOptions := stdout.commandOptions(stderr, operationOptions.Context)
	container, err := docker.RequireRunningProfileWithOptions(name, commandOptions)
	if err != nil {
		return result, err
	}

	containerDestination := temporaryContainerPath("backup")
	defer removeContainerPathWithOptions(container, containerDestination, commandOptions)

	fmt.Fprintf(stdout, "backing up database %q from profile %q to %q\n", database, name, destination)
	args := []string{
		"click-odoo-backupdb",
		"--log-level=error",
		"--format", format,
	}
	if force {
		args = append(args, "--force")
	}
	if ifExists {
		args = append(args, "--if-exists")
	}
	if filestore {
		args = append(args, "--filestore")
	} else {
		args = append(args, "--no-filestore")
	}
	args = append(args, database, containerDestination)

	if err := runWithCommandOptions(container, commandOptions, args...); err != nil {
		return result, fmt.Errorf("backup database %q: %w", database, err)
	}
	if ifExists && !containerPathExistsWithOptions(container, containerDestination, commandOptions) {
		fmt.Fprintf(stdout, "database %q does not exist; backup skipped\n", database)
		result.Skipped = true
		return result, nil
	}
	if err := copyBackupToHost(container, containerDestination, destination, force, commandOptions); err != nil {
		return result, fmt.Errorf("write backup %q: %w", destination, err)
	}
	fmt.Fprintf(stdout, "database %q backed up to %q\n", database, destination)
	return result, nil
}

func validateBackupFormat(format string) error {
	if !backupFormats[format] {
		return fmt.Errorf("invalid backup format %q: use zip, dump, or folder", format)
	}
	return nil
}

func defaultBackupDestination(database string) string {
	now := time.Now()
	filename := fmt.Sprintf("%s_%s_%d.zip", database, now.Format("060102"), now.Unix())
	return filepath.Join("backups", filename)
}

func validateBackupDestination(destination string) error {
	if filepath.Dir(destination) == destination {
		return fmt.Errorf("refusing to use filesystem root as backup destination")
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("inspect current directory: %w", err)
	}
	if destination == filepath.Clean(workingDirectory) {
		return fmt.Errorf("refusing to use current directory as backup destination")
	}
	return nil
}

func copyBackupToHost(container, source, destination string, force bool, options docker.CommandOptions) error {
	parent := filepath.Dir(destination)
	info, err := os.Stat(parent)
	if err != nil {
		return fmt.Errorf("inspect destination directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("destination parent %q is not a directory", parent)
	}

	staging := temporaryLocalPath(destination)
	if err := docker.CopyFromWithOptions(container, source, staging, options); err != nil {
		return fmt.Errorf("copy from profile: %w", err)
	}
	defer os.RemoveAll(staging)

	if _, err := os.Lstat(destination); err == nil {
		if !force {
			return fmt.Errorf("destination already exists: %s", destination)
		}
		if err := os.RemoveAll(destination); err != nil {
			return fmt.Errorf("remove existing destination: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect destination: %w", err)
	}

	if err := os.Rename(staging, destination); err != nil {
		return fmt.Errorf("move staged backup into place: %w", err)
	}
	return nil
}

func absolutePath(path, description string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("%s is required", description)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", description, err)
	}
	return filepath.Clean(absolute), nil
}

func temporaryContainerPath(kind string) string {
	return fmt.Sprintf("/tmp/lidoo-%s-%d-%d", kind, os.Getpid(), time.Now().UnixNano())
}

func temporaryLocalPath(destination string) string {
	return fmt.Sprintf("%s.lidoo-%d-%d", destination, os.Getpid(), time.Now().UnixNano())
}

func containerPathExists(container, path string) bool {
	return containerPathExistsWithOptions(container, path, docker.CommandOptions{})
}

func containerPathExistsWithOptions(container, path string, options docker.CommandOptions) bool {
	return docker.ExecWithOptions(container, options, "test", "-e", path) == nil
}

func removeContainerPath(container, path string) {
	removeContainerPathWithOptions(container, path, docker.CommandOptions{})
}

func removeContainerPathWithOptions(container, path string, options docker.CommandOptions) {
	_ = docker.ExecAsUserWithOptions(container, "root", options, "rm", "-rf", "--", path)
}
