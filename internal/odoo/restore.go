package odoo

import (
	"fmt"
	"os"

	"lidoo/internal/docker"
	"lidoo/internal/files"
)

func Restore(name, database, source string, copyDatabase, force, neutralize bool, jobs int, state files.State) error {
	database, err := resolveDatabaseName(state, name, database)
	if err != nil {
		return err
	}
	if jobs < 1 {
		return fmt.Errorf("restore jobs must be at least 1")
	}
	source, err = absolutePath(source, "restore source")
	if err != nil {
		return err
	}
	if _, err := os.Stat(source); err != nil {
		return fmt.Errorf("restore source %q: %w", source, err)
	}

	container, err := docker.RequireRunningProfile(name)
	if err != nil {
		return err
	}

	containerSource := temporaryContainerPath("restore")
	defer removeContainerPath(container, containerSource)

	fmt.Printf("restoring database %q in profile %q from %q\n", database, name, source)
	if err := docker.CopyTo(container, source, containerSource); err != nil {
		return fmt.Errorf("copy restore source to profile: %w", err)
	}
	if err := docker.ExecAsUser(container, "root", "chown", "-R", "odoo", containerSource); err != nil {
		return fmt.Errorf("make restore source readable: %w", err)
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

	if err := run(container, args...); err != nil {
		return fmt.Errorf("restore database %q: %w", database, err)
	}
	fmt.Printf("database %q restored\n", database)
	return nil
}
