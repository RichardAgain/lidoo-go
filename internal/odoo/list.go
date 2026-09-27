package odoo

import (
	"fmt"
	"regexp"
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

// ListDatabases returns the databases visible through a profile, following its
// effective database filter mode: prefix-scoped names in "profile" mode, every
// database in "disabled" mode, and the configured pattern in "custom" mode.
func ListDatabases(name string, state files.State) ([]Database, error) {
	if name == "" {
		return nil, fmt.Errorf("database list requires name")
	}
	if err := docker.ValidateProfileName(name); err != nil {
		return nil, err
	}
	config, found, err := profiles.Lookup(state, name)
	if err != nil {
		return nil, fmt.Errorf("read profile %q: %w", name, err)
	}
	prefix := name + "__"
	mode := profiles.DBFilterModeProfile
	pattern := ""
	if found {
		prefix = config.Prefix
		mode = config.EffectiveDBFilterMode()
		pattern = config.DBFilterPattern
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
	return ParseDatabases(string(output), prefix, mode, pattern)
}

var protectedDatabases = map[string]bool{
	"postgres":  true,
	"template0": true,
	"template1": true,
}

// ParseDatabases turns the raw psql listing into logical/physical pairs for the
// given filter mode. System databases are always excluded.
func ParseDatabases(output, prefix, mode, pattern string) ([]Database, error) {
	logicalFor, err := databaseMatcher(prefix, mode, pattern)
	if err != nil {
		return nil, err
	}

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
		if protectedDatabases[physical] {
			continue
		}
		// click-odoo-initdb keeps its template cache in cache-<...> databases;
		// they are internal and should not be managed as user databases.
		if strings.HasPrefix(physical, "cache-") {
			continue
		}
		logical, ok := logicalFor(physical)
		if !ok {
			continue
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

func databaseMatcher(prefix, mode, pattern string) (func(string) (string, bool), error) {
	switch mode {
	case profiles.DBFilterModeDisabled:
		return func(physical string) (string, bool) {
			return physical, true
		}, nil
	case profiles.DBFilterModeCustom:
		expression, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid database filter pattern: %w", err)
		}
		return func(physical string) (string, bool) {
			if !expression.MatchString(physical) {
				return "", false
			}
			return stripDatabasePrefix(physical, prefix), true
		}, nil
	default:
		return func(physical string) (string, bool) {
			if prefix != "" && !strings.HasPrefix(physical, prefix) {
				return "", false
			}
			return stripDatabasePrefix(physical, prefix), true
		}, nil
	}
}

func stripDatabasePrefix(physical, prefix string) string {
	if prefix != "" && strings.HasPrefix(physical, prefix) {
		return strings.TrimPrefix(physical, prefix)
	}
	return physical
}
