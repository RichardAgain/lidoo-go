package odoo

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"lidoo/internal/docker"
	"lidoo/internal/files"
	profiles "lidoo/internal/profile"
)

type Database struct {
	Logical  string
	Physical string
}

func ListDatabases(name string, state files.State) error {
	if name == "" {
		return fmt.Errorf("database list requires name")
	}
	if err := docker.ValidateProfileName(name); err != nil {
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

	output, err := runCapture(container,
		"psql",
		"--no-psqlrc",
		"--tuples-only",
		"--no-align",
		"--command", "SELECT datname FROM pg_database WHERE datallowconn ORDER BY datname",
		"postgres",
	)
	if err != nil {
		return fmt.Errorf("list databases for profile %q: %w", name, err)
	}
	databases, err := ParseDatabases(string(output), prefix)
	if err != nil {
		return err
	}
	return renderDatabases(databases, prefix, os.Stdout)
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

func renderDatabases(databases []Database, prefix string, output io.Writer) error {
	if len(databases) == 0 {
		_, err := fmt.Fprintln(output, "no databases found")
		return err
	}

	table := tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
	if prefix == "" {
		fmt.Fprintln(table, "DATABASE")
		for _, database := range databases {
			fmt.Fprintln(table, database.Physical)
		}
	} else {
		fmt.Fprintln(table, "LOGICAL DATABASE\tPHYSICAL DATABASE")
		for _, database := range databases {
			fmt.Fprintf(table, "%s\t%s\n", database.Logical, database.Physical)
		}
	}
	return table.Flush()
}
