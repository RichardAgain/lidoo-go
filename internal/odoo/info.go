package odoo

import (
	"fmt"
	"strings"

	"lidoo/internal/docker"
	"lidoo/internal/files"
)

type DatabaseInfo struct {
	Logical   string
	Physical  string
	Size      string
	Owner     string
	Available bool
}

func InfoDatabase(name, database string, state files.State) error {
	physical, err := resolveDatabaseName(state, name, database)
	if err != nil {
		return err
	}
	prefix, err := profilePrefix(state, name)
	if err != nil {
		return err
	}
	container, err := docker.RequireRunningProfile(name)
	if err != nil {
		return err
	}

	query := "SELECT datname, pg_size_pretty(pg_database_size(datname)), pg_get_userbyid(datdba) FROM pg_database WHERE datname = " + postgresString(physical)
	output, err := runCapture(container,
		"psql", "--no-psqlrc", "--tuples-only", "--no-align", "--field-separator", "\t",
		"--command", query, "postgres",
	)
	if err != nil {
		return fmt.Errorf("inspect database %q: %w", physical, err)
	}
	info, found, err := parseDatabaseInfo(string(output), prefix, physical)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("database %q does not exist in profile %q", database, name)
	}

	_, connectionErr := runCapture(container,
		"psql", "--no-psqlrc", "--command", "SELECT 1", physical,
	)
	info.Available = connectionErr == nil

	fmt.Printf("logical database: %s\n", info.Logical)
	fmt.Printf("physical database: %s\n", info.Physical)
	fmt.Printf("size: %s\n", info.Size)
	fmt.Printf("owner: %s\n", info.Owner)
	if info.Available {
		fmt.Println("connection: available")
	} else {
		fmt.Println("connection: unavailable")
	}
	return nil
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
