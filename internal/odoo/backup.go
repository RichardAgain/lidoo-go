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

func Backup(name, database, destination, format string, force, ifExists, filestore bool, state files.State) error {
	databaseName := database
	database, err := resolveDatabaseName(state, name, database)
	if err != nil {
		return err
	}
	if err := validateBackupFormat(format); err != nil {
		return err
	}
	if destination == "" {
		if format != "zip" {
			return fmt.Errorf("a backup destination is required when using format %q", format)
		}
		destination = defaultBackupDestination(databaseName)
	}
	destination, err = absolutePath(destination, "backup destination")
	if err != nil {
		return err
	}
	if err := validateBackupDestination(destination); err != nil {
		return err
	}
	if !force && !ifExists {
		if _, err := os.Lstat(destination); err == nil {
			return fmt.Errorf("destination already exists: %s", destination)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect destination: %w", err)
		}
	}

	container, err := docker.RequireRunningProfile(name)
	if err != nil {
		return err
	}

	containerDestination := temporaryContainerPath("backup")
	defer removeContainerPath(container, containerDestination)

	fmt.Printf("backing up database %q from profile %q to %q\n", database, name, destination)
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

	if err := run(container, args...); err != nil {
		return fmt.Errorf("backup database %q: %w", database, err)
	}
	if ifExists && !containerPathExists(container, containerDestination) {
		fmt.Printf("database %q does not exist; backup skipped\n", database)
		return nil
	}
	if err := copyBackupToHost(container, containerDestination, destination, force); err != nil {
		return fmt.Errorf("write backup %q: %w", destination, err)
	}
	fmt.Printf("database %q backed up to %q\n", database, destination)
	return nil
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

func copyBackupToHost(container, source, destination string, force bool) error {
	parent := filepath.Dir(destination)
	info, err := os.Stat(parent)
	if err != nil {
		return fmt.Errorf("inspect destination directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("destination parent %q is not a directory", parent)
	}

	staging := temporaryLocalPath(destination)
	if err := docker.CopyFrom(container, source, staging); err != nil {
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
	return docker.Exec(container, "test", "-e", path) == nil
}

func removeContainerPath(container, path string) {
	_ = docker.ExecAsUser(container, "root", "rm", "-rf", "--", path)
}
