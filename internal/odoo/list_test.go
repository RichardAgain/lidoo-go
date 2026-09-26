package odoo

import (
	"testing"

	"lidoo/internal/profile"
)

func TestParseDatabasesProfileModeKeepsPrefix(t *testing.T) {
	output := "other_db\npostgres\nsmoke__alpha\nsmoke__beta\n"
	databases, err := ParseDatabases(output, "smoke__", profile.DBFilterModeProfile, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(databases) != 2 {
		t.Fatalf("databases = %+v", databases)
	}
	if databases[0].Logical != "alpha" || databases[0].Physical != "smoke__alpha" {
		t.Fatalf("first = %+v", databases[0])
	}
}

func TestParseDatabasesDisabledModeShowsEverything(t *testing.T) {
	output := "other_db\npostgres\ntemplate1\nsmoke__alpha\n"
	databases, err := ParseDatabases(output, "smoke__", profile.DBFilterModeDisabled, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(databases) != 2 {
		t.Fatalf("databases = %+v", databases)
	}
	if databases[0].Logical != "other_db" || databases[0].Physical != "other_db" {
		t.Fatalf("first = %+v", databases[0])
	}
	if databases[1].Physical != "smoke__alpha" {
		t.Fatalf("second = %+v", databases[1])
	}
}

func TestParseDatabasesCustomModeUsesPattern(t *testing.T) {
	output := "smoke__dev\nsmoke__prod\nsmoke__staging\n"
	databases, err := ParseDatabases(output, "smoke__", profile.DBFilterModeCustom, `^smoke__(dev|staging)$`)
	if err != nil {
		t.Fatal(err)
	}
	if len(databases) != 2 {
		t.Fatalf("databases = %+v", databases)
	}
	if databases[0].Logical != "dev" || databases[1].Logical != "staging" {
		t.Fatalf("databases = %+v", databases)
	}
}

func TestParseDatabasesRejectsInvalidPattern(t *testing.T) {
	if _, err := ParseDatabases("smoke__a\n", "smoke__", profile.DBFilterModeCustom, "["); err == nil {
		t.Fatal("expected an error for an invalid custom pattern")
	}
}
