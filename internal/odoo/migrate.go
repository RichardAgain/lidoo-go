package odoo

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"lidoo/internal/addons"
	"lidoo/internal/docker"
	"lidoo/internal/files"
	profiles "lidoo/internal/profile"
)

// communityAddonsPath is the Community addons directory inside the Odoo image.
// The stock image ships Community only; Enterprise addons are attached to the
// profile and mounted under /opt/addons/<name>.
const communityAddonsPath = "/usr/lib/python3/dist-packages/odoo/addons"

// openUpgradeFrameworkModule is the OCA framework module loaded by Odoo during
// the upgrade so the OpenUpgrade migration scripts run for Community modules.
const openUpgradeFrameworkModule = "openupgrade_framework"

// Migrate upgrades a copy of a source database with the target profile's Odoo.
//
// It reuses the existing backup and restore operations to move the source
// database into the target profile, then runs Odoo's own module upgrade inside
// the target container:
//
//	odoo --addons-path=<...> --load=base,web[,openupgrade_framework] \
//	     --database <target> --update all --stop-after-init --no-http
//
// `--update all` executes every installed module's own migration scripts, which
// covers Community and Enterprise; loading the OpenUpgrade framework only adds
// the extra Community scripts. `communityOnly` narrows the addons path to the
// Community directory so a profile that has Enterprise attached can be migrated
// as Community. No external SQL is involved: the whole migration is Odoo's own
// ORM upgrade plus the existing backup/restore commands.
//
// `openUpgradeAddon` names an attached addon (mounted at /opt/addons/<name>)
// holding the OpenUpgrade repository; it is optional, and without it only
// Odoo's own migration scripts run.
func Migrate(sourceProfile, sourceDatabase, targetProfile, targetDatabase string,
	communityOnly bool, openUpgradeAddon string, force bool, state files.State,
	options ...OperationOptions) (result OperationResult, err error) {
	stdout, stderr, operationOptions := newOperationStreams(options)
	result.ProfileName = targetProfile
	defer func() {
		result.Output = stdout.String()
		result.ErrorOutput = stderr.String()
	}()

	if err := validateDatabaseOperationInputs(sourceProfile, sourceDatabase); err != nil {
		return result, err
	}
	if err := validateDatabaseOperationInputs(targetProfile, targetDatabase); err != nil {
		return result, err
	}
	if sourceProfile == targetProfile {
		return result, errors.New("source and target profiles must differ")
	}
	if err := validateDropDatabase(targetDatabase); err != nil {
		return result, err
	}

	targetPhysical, err := resolveDatabaseName(state, targetProfile, targetDatabase)
	if err != nil {
		return result, err
	}
	result.LogicalDatabase = targetDatabase
	result.PhysicalDatabase = targetPhysical

	// The source database must exist in the source profile.
	if _, err := resolveExistingDatabaseName(state, sourceProfile, sourceDatabase); err != nil {
		return result, err
	}

	// 1. Back up the source database, filestore included, to a host temporary
	//    zip. Reusing Backup keeps the container path and the copy-out logic in
	//    one place.
	backupPath := filepath.Join(os.TempDir(),
		fmt.Sprintf("lidoo-migrate-%s-%d.zip", targetDatabase, time.Now().UnixNano()))
	defer os.Remove(backupPath)
	fmt.Fprintf(stdout, "backing up %q from profile %q\n", sourceDatabase, sourceProfile)
	if _, err := Backup(sourceProfile, sourceDatabase, backupPath, "zip", true, false, true, state,
		operationOptions); err != nil {
		return result, fmt.Errorf("backup source database: %w", err)
	}

	// 2. Restore it as a copy inside the target profile.
	fmt.Fprintf(stdout, "restoring into profile %q as %q\n", targetProfile, targetDatabase)
	if _, err := Restore(targetProfile, targetDatabase, backupPath, true, force, false, 1, state,
		operationOptions); err != nil {
		return result, fmt.Errorf("restore into target profile: %w", err)
	}

	// 3. Run the core upgrade in the target container.
	commandOptions := stdout.commandOptions(stderr, operationOptions.Context)
	container, err := docker.RequireRunningProfileWithOptions(targetProfile, commandOptions)
	if err != nil {
		return result, err
	}
	addonsPath, err := migrationAddonsPath(state, targetProfile, communityOnly, openUpgradeAddon)
	if err != nil {
		return result, err
	}
	args := []string{
		"odoo",
		"--addons-path=" + addonsPath,
		"--load=" + migrationLoadModules(openUpgradeAddon),
		"--database", targetPhysical,
		"--update", "all",
		"--stop-after-init",
		"--no-http",
	}
	fmt.Fprintf(stdout, "upgrading database %q in profile %q\n", targetPhysical, targetProfile)
	if err := runWithCommandOptions(container, commandOptions, args...); err != nil {
		return result, fmt.Errorf("upgrade database %q: %w", targetPhysical, err)
	}
	fmt.Fprintf(stdout, "database %q migrated with the target profile's Odoo\n", targetPhysical)
	return result, nil
}

// migrationAddonsPath builds the container addons path for the upgrade: the
// Community directory, the profile's attached addons unless `communityOnly`,
// and the OpenUpgrade checkout when given.
func migrationAddonsPath(state files.State, profileName string, communityOnly bool, openUpgradeAddon string) (string, error) {
	paths := []string{communityAddonsPath}
	seen := map[string]bool{communityAddonsPath: true}
	add := func(path string) {
		if !seen[path] {
			seen[path] = true
			paths = append(paths, path)
		}
	}
	if !communityOnly {
		config, found, err := profiles.Lookup(state, profileName)
		if err != nil {
			return "", fmt.Errorf("read profile %q: %w", profileName, err)
		}
		if found {
			mounts, err := addons.ResolveMounts(state, config.Addons)
			if err != nil {
				return "", fmt.Errorf("resolve profile addons: %w", err)
			}
			for _, mount := range mounts {
				add("/opt/addons/" + mount.Name)
			}
		}
	}
	if openUpgradeAddon != "" {
		// Resolving it here validates the addon is registered before the
		// backup/restore work starts. The OCA repository keeps
		// openupgrade_framework and openupgrade_scripts at its root, so the
		// addons path is the mounted repository itself. Adding it through the
		// dedupe keeps a profile that already mounts OpenUpgrade from listing
		// the same directory twice.
		if _, err := addons.ResolveMounts(state, []string{openUpgradeAddon}); err != nil {
			return "", fmt.Errorf("resolve --openupgrade addon: %w", err)
		}
		add("/opt/addons/" + openUpgradeAddon)
	}
	return strings.Join(paths, ","), nil
}

func migrationLoadModules(openUpgradeAddon string) string {
	if openUpgradeAddon == "" {
		return "base,web"
	}
	return "base,web," + openUpgradeFrameworkModule
}
