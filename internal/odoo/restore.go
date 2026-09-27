package odoo

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"lidoo/internal/docker"
	"lidoo/internal/files"
)

// DeriveRestoreDestination turns a dump path into a fresh, non-colliding
// database name: the sanitized file base, suffixed with _restore when it
// already exists. Both the CLI and the TUI use it so a restore never silently
// targets an existing database.
func DeriveRestoreDestination(source string, existing []Database) string {
	base := sanitizeDatabaseName(strings.TrimSuffix(filepath.Base(source), filepath.Ext(source)))
	if base == "" {
		base = "restored"
	}
	if !databaseNameExists(base, existing) {
		return base
	}
	candidate := base + "_restore"
	for index := 2; databaseNameExists(candidate, existing); index++ {
		candidate = fmt.Sprintf("%s_restore_%d", base, index)
	}
	return candidate
}

func databaseNameExists(name string, existing []Database) bool {
	for _, database := range existing {
		if database.Logical == name || database.Physical == name {
			return true
		}
	}
	return false
}

func sanitizeDatabaseName(value string) string {
	var builder strings.Builder
	for _, character := range strings.ToLower(value) {
		switch {
		case character >= 'a' && character <= 'z',
			character >= '0' && character <= '9',
			character == '_', character == '-', character == '.':
			builder.WriteRune(character)
		default:
			builder.WriteRune('_')
		}
	}
	return strings.Trim(builder.String(), "_.-")
}

func Restore(name, database, source string, copyDatabase, force, neutralize bool, jobs int, state files.State, options ...OperationOptions) (result OperationResult, err error) {
	stdout, stderr, operationOptions := newOperationStreams(options)
	result.ProfileName = name
	defer func() {
		result.Output = stdout.String()
		result.ErrorOutput = stderr.String()
	}()

	logicalDatabase := database
	if found, resolveErr := findDatabase(state, name, database); resolveErr == nil {
		database = found.Physical
	} else {
		database, err = resolveDatabaseName(state, name, database)
		if err != nil {
			return result, err
		}
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
