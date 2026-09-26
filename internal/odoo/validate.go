package odoo

import (
	"errors"
	"fmt"
	"regexp"

	"lidoo/internal/docker"
	"lidoo/internal/files"
)

var databaseNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$`)

func validateDatabaseOperationInputs(profile, database string) error {
	if profile == "" {
		return errors.New("database operation requires name")
	}
	if err := docker.ValidateProfileName(profile); err != nil {
		return err
	}
	return validateDatabaseName(database)
}

func validateDatabaseName(name string) error {
	if !databaseNamePattern.MatchString(name) {
		return fmt.Errorf("invalid database name %q: use 1-63 letters, numbers, dots, underscores, or hyphens", name)
	}
	return nil
}

func validateDropDatabase(name string) error {
	switch name {
	case "postgres", "template0", "template1":
		return fmt.Errorf("refusing to drop protected database %q", name)
	default:
		return nil
	}
}

func findDatabase(state files.State, profileName, databaseName string) (Database, error) {
	if err := validateDatabaseOperationInputs(profileName, databaseName); err != nil {
		return Database{}, err
	}
	databases, err := ListDatabases(profileName, state)
	if err != nil {
		return Database{}, err
	}
	for _, database := range databases {
		if database.Logical == databaseName || database.Physical == databaseName {
			return database, nil
		}
	}
	return Database{}, &DatabaseNotFoundError{Profile: profileName, Database: databaseName}
}

func resolveExistingDatabaseName(state files.State, profileName, databaseName string) (string, error) {
	database, err := findDatabase(state, profileName, databaseName)
	if err != nil {
		return "", err
	}
	return database.Physical, nil
}
