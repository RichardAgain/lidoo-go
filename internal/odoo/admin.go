package odoo

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"lidoo/internal/docker"
	"lidoo/internal/files"
)

// SetAdminPassword changes the password of the database's built-in admin user
// (base.user_admin, login "admin") through Odoo's own ORM. Running the change
// through Odoo keeps the password hashing correct across Odoo versions.
//
// The generated script is piped to `odoo shell` on standard input, so the
// password never appears in a process argument list.
func SetAdminPassword(name, database, password string, state files.State, options ...OperationOptions) (result OperationResult, err error) {
	stdout, stderr, operationOptions := newOperationStreams(options)
	result.ProfileName = name
	defer func() {
		result.Output = stdout.String()
		result.ErrorOutput = stderr.String()
	}()

	if strings.TrimSpace(password) == "" {
		return result, errors.New("admin password cannot be empty")
	}

	logicalDatabase := database
	database, err = resolveExistingDatabaseName(state, name, database)
	if err != nil {
		return result, err
	}
	result.LogicalDatabase = logicalDatabase
	result.PhysicalDatabase = database

	commandOptions := stdout.commandOptions(stderr, operationOptions.Context)
	container, err := docker.RequireRunningProfileWithOptions(name, commandOptions)
	if err != nil {
		return result, err
	}

	fmt.Fprintf(stdout, "setting admin password for database %q in profile %q\n", database, name)
	encoded := base64.StdEncoding.EncodeToString([]byte(password))
	script := strings.Join([]string{
		"import base64",
		"admin = env.ref('base.user_admin')",
		"admin.write({'password': base64.b64decode('" + encoded + "').decode('utf-8')})",
		"env.cr.commit()",
		"print('admin password updated')",
		"",
	}, "\n")
	args := []string{"odoo", "shell", "--no-http", "--log-level=error", "-d", database}
	if err := runWithInputAndOptions(container, script, commandOptions, args...); err != nil {
		return result, fmt.Errorf("set admin password for database %q: %w", database, err)
	}
	fmt.Fprintf(stdout, "admin password updated for database %q\n", database)
	return result, nil
}
