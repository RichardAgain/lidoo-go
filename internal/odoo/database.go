package odoo

import (
	"fmt"

	"lidoo/internal/files"
)

func resolveDatabaseName(state files.State, profile, database string) (string, error) {
	if err := validateDatabaseOperationInputs(profile, database); err != nil {
		return "", err
	}

	prefix, err := files.ContainerPrefix(state, profile)
	if err != nil {
		return "", fmt.Errorf("read database prefix for profile %q: %w", profile, err)
	}

	qualified := prefix + database
	if err := validateDatabaseName(qualified); err != nil {
		return "", fmt.Errorf("invalid database name %q after applying profile prefix: %w", qualified, err)
	}
	return qualified, nil
}
