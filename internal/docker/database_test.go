package docker

import (
	"strings"
	"testing"
)

func TestValidateDatabaseName(t *testing.T) {
	tests := []struct {
		name  string
		valid bool
	}{
		{name: "testing_db", valid: true},
		{name: "demo-18", valid: true},
		{name: "client.test", valid: true},
		{name: "", valid: false},
		{name: "with space", valid: false},
		{name: ";drop", valid: false},
		{name: "-demo", valid: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDatabaseName(tt.name)
			if (err == nil) != tt.valid {
				t.Fatalf("validateDatabaseName(%q) error = %v, valid = %v", tt.name, err, tt.valid)
			}
		})
	}
}

func TestDatabaseCommand(t *testing.T) {
	tests := []struct {
		name       string
		operation  string
		database   string
		modules    string
		updateAll  bool
		binary     string
		databaseAt string
	}{
		{
			name:       "init",
			operation:  databaseOperationInit,
			database:   "testing_db",
			modules:    "base,sale",
			binary:     "click-odoo-initdb",
			databaseAt: "--new-database \"$1\"",
		},
		{
			name:       "update",
			operation:  databaseOperationUpdate,
			database:   "testing_db",
			binary:     "click-odoo-update",
			databaseAt: "--database \"$1\"",
		},
		{
			name:       "update all",
			operation:  databaseOperationUpdate,
			database:   "testing_db",
			updateAll:  true,
			binary:     "click-odoo-update",
			databaseAt: "--database \"$1\"",
		},
		{
			name:       "drop",
			operation:  databaseOperationDrop,
			database:   "testing_db",
			binary:     "click-odoo-dropdb",
			databaseAt: "\"$1\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := databaseCommand("lidoo-testing", tt.operation, tt.database, tt.modules, tt.updateAll)
			if len(args) < 7 {
				t.Fatalf("databaseCommand returned too few args: %v", args)
			}
			if got, want := strings.Join(args[:4], " "), "exec lidoo-testing sh -c"; got != want {
				t.Fatalf("docker exec prefix = %q, want %q", got, want)
			}
			script := args[4]
			if !strings.Contains(script, tt.binary) {
				t.Errorf("script missing %q: %q", tt.binary, script)
			}
			if !strings.Contains(script, "command -v "+tt.binary) {
				t.Errorf("script must check that %q is installed: %q", tt.binary, script)
			}
			if !strings.Contains(script, "click-odoo-contrib") {
				t.Errorf("script must explain how to recover an old image: %q", script)
			}
			if !strings.Contains(script, `export PGPASSWORD="${POSTGRES_PASSWORD:?`) {
				t.Errorf("script must require the database password from the environment: %q", script)
			}
			if !strings.Contains(script, tt.databaseAt) {
				t.Errorf("script missing database argument %q: %q", tt.databaseAt, script)
			}
			if tt.updateAll && !strings.Contains(script, "--update-all") {
				t.Errorf("update-all script missing --update-all: %q", script)
			}
			if strings.Contains(script, tt.database) || (tt.modules != "" && strings.Contains(script, tt.modules)) {
				t.Errorf("user values must be positional arguments, not shell script text: %q", script)
			}
			databaseIndex := len(args) - 1
			if tt.operation == databaseOperationInit {
				databaseIndex--
			}
			if got, want := args[databaseIndex], tt.database; got != want {
				t.Errorf("database positional argument = %q, want %q", got, want)
			}
			if tt.operation == databaseOperationInit {
				if got, want := args[databaseIndex+1], tt.modules; got != want {
					t.Errorf("modules positional argument = %q, want %q", got, want)
				}
			}
		})
	}
}

func TestValidateDropDatabaseProtectsSystemDatabases(t *testing.T) {
	for _, name := range []string{"postgres", "template0", "template1"} {
		if err := validateDropDatabase(name); err == nil {
			t.Errorf("validateDropDatabase(%q) should reject system database", name)
		}
	}
	if err := validateDropDatabase("testing_db"); err != nil {
		t.Fatalf("validateDropDatabase(testing_db) error = %v", err)
	}
}

func TestDropRequiresExplicitConfirmation(t *testing.T) {
	err := Drop([]string{"--name", "testing", "--database", "testing_db"})
	if err == nil || !strings.Contains(err.Error(), "requires --yes") {
		t.Fatalf("Drop without confirmation error = %v, want --yes error", err)
	}
}

func TestDropRejectsProtectedDatabaseBeforeDocker(t *testing.T) {
	err := Drop([]string{"--name", "testing", "--database", "postgres", "--yes"})
	if err == nil || !strings.Contains(err.Error(), "protected database") {
		t.Fatalf("Drop of postgres error = %v, want protected database error", err)
	}
}
