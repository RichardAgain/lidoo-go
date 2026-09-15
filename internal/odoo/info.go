package odoo

import (
	"errors"
	"fmt"
	"strings"

	"lidoo/internal/docker"
	"lidoo/internal/files"
)

// ErrDatabaseNotFound identifies an action that targeted a database which no
// longer exists in its profile.
var ErrDatabaseNotFound = errors.New("database not found")

// DatabaseNotFoundError retains the profile and logical database involved in a
// failed lookup while remaining detectable with errors.Is.
type DatabaseNotFoundError struct {
	Profile  string
	Database string
}

func (err *DatabaseNotFoundError) Error() string {
	return fmt.Sprintf("database %q does not exist in profile %q", err.Database, err.Profile)
}

func (err *DatabaseNotFoundError) Unwrap() error {
	return ErrDatabaseNotFound
}

type DatabaseInfo struct {
	Logical   string
	Physical  string
	Size      string
	Owner     string
	Available bool
}

// InfoDatabase returns inspection data for a profile database without writing
// to the terminal.
func InfoDatabase(name, database string, state files.State) (DatabaseInfo, error) {
	physical, err := resolveDatabaseName(state, name, database)
	if err != nil {
		return DatabaseInfo{}, err
	}
	prefix, err := profilePrefix(state, name)
	if err != nil {
		return DatabaseInfo{}, err
	}
	container, err := docker.RequireRunningProfile(name)
	if err != nil {
		return DatabaseInfo{}, err
	}

	query := "SELECT datname, pg_size_pretty(pg_database_size(datname)), pg_get_userbyid(datdba) FROM pg_database WHERE datname = " + postgresString(physical)
	output, err := runCapture(container,
		"psql", "--no-psqlrc", "--tuples-only", "--no-align", "--field-separator", "\t",
		"--command", query, "postgres",
	)
	if err != nil {
		return DatabaseInfo{}, fmt.Errorf("inspect database %q: %w", physical, err)
	}
	info, found, err := parseDatabaseInfo(string(output), prefix, physical)
	if err != nil {
		return DatabaseInfo{}, err
	}
	if !found {
		return DatabaseInfo{}, &DatabaseNotFoundError{Profile: name, Database: database}
	}

	_, connectionErr := runCapture(container,
		"psql", "--no-psqlrc", "--command", "SELECT 1", physical,
	)
	info.Available = connectionErr == nil
	return info, nil
}

func parseDatabaseInfo(output, prefix, physical string) (DatabaseInfo, bool, error) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != 3 {
			return DatabaseInfo{}, false, fmt.Errorf("invalid database info from PostgreSQL: %q", line)
		}
		if strings.TrimSpace(fields[0]) != physical {
			continue
		}
		logical := physical
		if prefix != "" {
			logical = strings.TrimPrefix(physical, prefix)
		}
		return DatabaseInfo{
			Logical:  logical,
			Physical: physical,
			Size:     strings.TrimSpace(fields[1]),
			Owner:    strings.TrimSpace(fields[2]),
		}, true, nil
	}
	return DatabaseInfo{}, false, nil
}

func ShellDatabase(name, database string, state files.State) error {
	physical, err := resolveDatabaseName(state, name, database)
	if err != nil {
		return err
	}
	container, err := docker.RequireRunningProfile(name)
	if err != nil {
		return err
	}
	return runInteractive(container, "psql", physical)
}

func postgresString(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
