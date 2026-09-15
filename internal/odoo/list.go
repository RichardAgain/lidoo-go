package odoo

import (
	"fmt"
	"sort"
	"strings"

	"lidoo/internal/docker"
	"lidoo/internal/files"
	profiles "lidoo/internal/profile"
)

type Database struct {
	Logical  string
	Physical string
}

// ListDatabases returns the databases visible through a profile's configured
// prefix. Logical and physical names remain separate for prefixed profiles.
func ListDatabases(name string, state files.State) ([]Database, error) {
	if name == "" {
		return nil, fmt.Errorf("database list requires name")
	}
	if err := docker.ValidateProfileName(name); err != nil {
		return nil, err
	}
	prefix, err := profilePrefix(state, name)
	if err != nil {
		return nil, err
	}
	container, err := docker.RequireRunningProfile(name)
	if err != nil {
		return nil, err
	}

	output, err := runCapture(container,
		"psql",
		"--no-psqlrc",
		"--tuples-only",
		"--no-align",
		"--command", "SELECT datname FROM pg_database WHERE datallowconn ORDER BY datname",
		"postgres",
	)
	if err != nil {
		return nil, fmt.Errorf("list databases for profile %q: %w", name, err)
	}
	return ParseDatabases(string(output), prefix)
}

func profilePrefix(state files.State, name string) (string, error) {
	prefix, err := profiles.Prefix(state, name)
	if err != nil {
		return "", fmt.Errorf("read database prefix for profile %q: %w", name, err)
	}
	return prefix, nil
}

func ParseDatabases(output, prefix string) ([]Database, error) {
	databases := make([]Database, 0)
	seen := make(map[string]bool)
	for _, line := range strings.Split(output, "\n") {
		physical := strings.TrimSpace(line)
		if physical == "" {
			continue
		}
		if strings.ContainsAny(physical, "\r\t") {
			return nil, fmt.Errorf("invalid database name from PostgreSQL: %q", physical)
		}
		if prefix != "" && !strings.HasPrefix(physical, prefix) {
			continue
		}
		logical := physical
		if prefix != "" {
			logical = strings.TrimPrefix(physical, prefix)
		}
		if seen[physical] {
			continue
		}
		seen[physical] = true
		databases = append(databases, Database{Logical: logical, Physical: physical})
	}
	sort.Slice(databases, func(i, j int) bool {
		return databases[i].Physical < databases[j].Physical
	})
	return databases, nil
}
