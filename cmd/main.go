package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"lidoo/internal/addons"
	"lidoo/internal/app"
	"lidoo/internal/docker"
	"lidoo/internal/files"
	"lidoo/internal/odoo"
	"lidoo/internal/profileio"
	"lidoo/internal/tui"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	command := os.Args[1]
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(os.Stderr)

	var name, version, database, modules, format string
	var updateAll, yes, force, ifExists, filestore, noFilestore bool
	var copyDatabase, move, neutralize, follow, waitForReady bool
	var jobs, tail int
	var waitTimeout time.Duration

	flags.StringVar(&name, "name", "", "container name")
	flags.StringVar(&version, "version", "", "Odoo version")
	flags.StringVar(&database, "database", "", "database name")
	flags.StringVar(&modules, "modules", "base", "comma-separated modules to install")
	flags.BoolVar(&updateAll, "update-all", false, "force a complete module update")
	flags.BoolVar(&yes, "y", false, "confirm to all")
	flags.BoolVar(&yes, "yes", false, "confirm to all")
	flags.BoolVar(&force, "force", false, "overwrite an existing backup or database")
	flags.BoolVar(&ifExists, "if-exists", false, "skip backup if the database does not exist")
	flags.StringVar(&format, "format", "zip", "backup format: zip, dump, or folder")
	flags.BoolVar(&filestore, "filestore", true, "include the filestore in a backup")
	flags.BoolVar(&noFilestore, "no-filestore", false, "exclude the filestore from a backup")
	flags.BoolVar(&copyDatabase, "copy", true, "restore as a database copy")
	flags.BoolVar(&move, "move", false, "restore by moving the database")
	flags.BoolVar(&neutralize, "neutralize", false, "neutralize a restored database")
	flags.IntVar(&jobs, "jobs", 1, "parallel jobs for folder restores")
	flags.BoolVar(&follow, "follow", false, "follow profile logs")
	flags.IntVar(&tail, "tail", -1, "number of log lines to show")
	flags.BoolVar(&waitForReady, "wait", false, "wait until the profile is ready")
	flags.DurationVar(&waitTimeout, "timeout", 2*time.Minute, "readiness timeout")

	if err := flags.Parse(os.Args[2:]); err != nil {
		os.Exit(2)
	}
	positional := flags.Args()

	if err := validatePositionals(command, positional); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	profileLifecycle := isProfileLifecycleCommand(command)
	databaseMutation := isDatabaseMutationCommand(command)
	mutatesState := commandMutatesState(command, positional)
	if profileLifecycle || databaseMutation {
		mutatesState = false
	}
	var err error
	var state files.State
	var stateLock *files.StateLock
	var service *app.Service
	var readStateErr bool
	if profileLifecycle || databaseMutation {
		service, err = app.Open()
	} else if command != "tui" {
		if mutatesState {
			stateLock, err = files.AcquireStateLock()
		}
		if err == nil {
			state, err = files.ReadState()
			readStateErr = err != nil
		}
	}
	if err != nil {
		if stateLock != nil {
			_ = stateLock.Close()
		}
		if readStateErr {
			fmt.Fprintln(os.Stderr, "read state:", err)
		} else {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}

	profileOptions := app.ProfileOperationOptions{
		Output:      os.Stdout,
		ErrorOutput: os.Stderr,
	}
	databaseOptions := app.DatabaseOperationOptions{
		Output:      os.Stdout,
		ErrorOutput: os.Stderr,
	}
	if profileLifecycle {
		profileOptions.Confirm = confirmProfileRemoval
	}

	exitCode := 0

	switch command {
	case "tui":
		err = tui.Run(context.Background())
	case "list":
		var profiles []docker.ProfileSummary
		profiles, err = docker.ListProfiles(state)
		if err == nil {
			err = renderProfileList(os.Stdout, profiles)
		}
	case "status":
		var details []docker.ProfileDetail
		details, err = docker.ProfileDetails(name, state)
		if err == nil {
			err = renderProfileDetails(details)
		}
	case "logs":
		err = docker.Logs(name, follow, tail)
	case "init":
		_, err = service.InitializeDatabase(context.Background(), name, database, modules, databaseOptions)
	case "update":
		_, err = service.UpdateDatabase(context.Background(), name, database, updateAll, databaseOptions)
	case "drop":
		_, err = service.DropDatabase(context.Background(), name, database, yes, databaseOptions)
	case "backup":
		backupPath := ""
		if len(positional) == 1 {
			backupPath = positional[0]
		}
		_, err = service.BackupDatabase(context.Background(), name, database, backupPath, format, force, ifExists, filestore && !noFilestore, databaseOptions)
	case "restore":
		_, err = service.RestoreDatabase(context.Background(), name, database, positional[0], copyDatabase && !move, force, neutralize, jobs, databaseOptions)
	case "run":
		err = service.RunProfile(context.Background(), app.RunProfileInput{Name: name, Version: version}, profileOptions)
		if err == nil && waitForReady {
			state, err = files.ReadState()
			if err == nil {
				err = odoo.Wait(name, waitTimeout, state)
			}
		}
	case "wait":
		err = odoo.Wait(name, waitTimeout, state)
	case "recreate":
		err = service.RecreateProfile(context.Background(), name, profileOptions)
	case "stop":
		err = service.StopProfile(context.Background(), name, profileOptions)
	case "restart":
		err = service.RestartProfile(context.Background(), name, profileOptions)
	case "remove":
		err = service.RemoveProfile(context.Background(), app.RemoveProfileInput{Name: name, Yes: yes}, profileOptions)
	case "profile":
		if len(positional) == 0 {
			err = errors.New("usage: lidoo profile export|import ...")
		} else {
			switch positional[0] {
			case "export":
				var profileName, destination string
				profileName, destination, err = parseProfileExportArgs(positional[1:], name)
				if err == nil {
					err = profileio.Export(profileName, destination, state)
				}
			case "import":
				var source string
				var overwrite bool
				source, overwrite, err = parseProfileImportArgs(positional[1:], yes)
				if err == nil {
					err = profileio.Import(source, overwrite, state)
				}
			default:
				err = fmt.Errorf("unknown profile command %q; use export or import", positional[0])
			}
		}
	case "db":
		if len(positional) == 0 {
			err = errors.New("usage: lidoo db list|info|shell --name <profile> [--database <database>]")
		} else {
			switch positional[0] {
			case "list":
				var profileName string
				profileName, err = parseDBArgs(positional[1:], name, "list")
				if err == nil {
					var databases []odoo.Database
					databases, err = odoo.ListDatabases(profileName, state)
					if err == nil {
						err = renderDatabaseList(databases)
					}
				}
			case "info", "shell":
				var profileName, databaseName string
				profileName, databaseName, err = parseDBArgsWithDatabase(positional[1:], name, "", positional[0])
				if err == nil && positional[0] == "info" {
					var info odoo.DatabaseInfo
					info, err = odoo.InfoDatabase(profileName, databaseName, state)
					if err == nil {
						err = renderDatabaseInfo(info)
					}
				} else if err == nil {
					err = odoo.ShellDatabase(profileName, databaseName, state)
				}
			default:
				err = fmt.Errorf("unknown db command %q; use list, info, or shell", positional[0])
			}
		}
	case "addons":
		switch {
		case len(positional) > 0 && positional[0] == "list":
			if len(positional) != 1 {
				err = errors.New("usage: lidoo addons list")
			} else {
				var statuses []addons.AddonStatus
				statuses, err = addons.List(state)
				if err == nil {
					err = renderAddonList(os.Stdout, statuses)
				}
			}
		case len(positional) > 0 && positional[0] == "status":
			if len(positional) > 2 {
				err = errors.New("usage: lidoo addons status [<addon name>]")
			} else if len(positional) == 2 {
				var status addons.AddonStatus
				status, err = addons.Status(positional[1], state)
				if err == nil {
					err = renderAddonStatus(os.Stdout, status)
				}
			} else {
				var statuses []addons.AddonStatus
				statuses, err = addons.List(state)
				if err == nil {
					err = renderAddonStatuses(os.Stdout, statuses)
				}
			}
		case len(positional) > 0 && positional[0] == "add":
			var addonName, url string
			var options addons.AddOptions
			addonName, url, options, err = parseAddonAddArgs(positional[1:])
			if err == nil {
				err = addons.Add(addonName, url, options, state)
			}
		case len(positional) > 0 && positional[0] == "attach":
			var profile string
			var addonNames []string
			var recreate bool
			profile, addonNames, recreate, err = parseAddonArgs(positional[1:], name, "attach")
			if err == nil {
				err = changeAddonMounts(profile, addonNames, recreate, true, state)
			}
		case len(positional) > 0 && positional[0] == "detach":
			var profile string
			var addonNames []string
			var recreate bool
			profile, addonNames, recreate, err = parseAddonArgs(positional[1:], name, "detach")
			if err == nil {
				err = changeAddonMounts(profile, addonNames, recreate, false, state)
			}
		case len(positional) > 0 && positional[0] == "rm":
			var addonName string
			var removeYes, removeForce bool
			addonName, removeYes, removeForce, err = parseAddonRemoveArgs(positional[1:], yes, false)
			if err == nil {
				err = addons.Remove(addonName, removeYes, removeForce, state)
			}
		case len(positional) > 0 && positional[0] == "worktree":
			var source, name, branch string
			var worktreeYes bool
			source, name, branch, worktreeYes, err = parseWorktreeArgs(positional[1:])
			if err == nil {
				err = addons.WorktreeWithConfirmation(source, name, branch, yes || worktreeYes, state)
			}
		case len(positional) > 0 && (positional[0] == "fetch" || positional[0] == "pull"):
			if len(positional) != 2 {
				err = fmt.Errorf("usage: lidoo addons %s <addon name>", positional[0])
			} else if positional[0] == "fetch" {
				err = addons.Fetch(positional[1], state)
			} else {
				err = addons.Pull(positional[1], state)
			}
		default:
			err = errors.New("usage: lidoo addons add <addon name> <git url> [--depth N] [--branch <branch>] | lidoo addons attach|detach --name <profile> <addon name> [<addon name> ...] | lidoo addons worktree <source> <name> --branch <branch> [--yes]")
		}
	default:
		usage()
		exitCode = 2
	}

	if err == nil && mutatesState {
		if saveErr := files.SaveState(state); saveErr != nil {
			fmt.Fprintln(os.Stderr, "save state:", saveErr)
			exitCode = 1
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		exitCode = docker.ExitCode(err)
	}
	if stateLock != nil {
		if closeErr := stateLock.Close(); closeErr != nil {
			fmt.Fprintln(os.Stderr, closeErr)
			exitCode = 1
		}
	}
	if exitCode != 0 {
		os.Exit(exitCode)
	}
}

func renderProfileList(output io.Writer, profiles []docker.ProfileSummary) error {
	if len(profiles) == 0 {
		_, err := fmt.Fprintln(output, "no profiles found")
		return err
	}

	table := tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
	fmt.Fprintln(table, "PROFILE\tSTATUS\tURL")
	for _, profile := range profiles {
		fmt.Fprintf(table, "%s\t%s\t%s\n", profile.Name, profile.State, profile.URL)
	}
	return table.Flush()
}

func renderProfileDetails(details []docker.ProfileDetail) error {
	if len(details) == 0 {
		fmt.Println("no profiles found")
		return nil
	}
	for index, detail := range details {
		if index > 0 {
			fmt.Println()
		}
		fmt.Printf("PROFILE: %s\n", detail.Name)
		fmt.Printf("docker state: %s\n", detail.DockerState)
		fmt.Printf("url: %s\n", detail.URL)
		fmt.Printf("odoo version: %s\n", detail.OdooVersion)
		fmt.Printf("attached addons: %s\n", strings.Join(detail.AttachedAddons, ", "))
		fmt.Printf("database prefix: %s\n", detail.DatabasePrefix)
		fmt.Printf("image: %s\n", detail.Image)
		fmt.Printf("filestore volume: %s\n", detail.FilestoreVolume)
		fmt.Printf("pending recreation: %s\n", detail.PendingRecreation.String())
	}
	return nil
}

func renderDatabaseList(databases []odoo.Database) error {
	if len(databases) == 0 {
		fmt.Println("no databases found")
		return nil
	}

	prefixed := false
	for _, database := range databases {
		if database.Logical != database.Physical {
			prefixed = true
			break
		}
	}
	table := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	if !prefixed {
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

func renderDatabaseInfo(info odoo.DatabaseInfo) error {
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

func renderAddonList(output io.Writer, statuses []addons.AddonStatus) error {
	if len(statuses) == 0 {
		_, err := fmt.Fprintln(output, "no addons found")
		return err
	}

	table := tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
	fmt.Fprintln(table, "NAME\tTYPE\tSOURCE/PARENT\tBRANCH\tPATH\tATTACHED PROFILES")
	for _, status := range statuses {
		source := status.Entry.Source
		if status.Kind == "worktree" {
			source = status.Entry.WorktreeOf
		}
		if source == "" {
			source = "-"
		}
		branch := status.Entry.Branch
		if branch == "" {
			branch = "-"
		}
		attached := strings.Join(status.AttachedProfiles, ",")
		if attached == "" {
			attached = "-"
		}
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\t%s\n", status.Name, status.Kind, source, branch, status.Path, attached)
	}
	return table.Flush()
}

func renderAddonStatuses(output io.Writer, statuses []addons.AddonStatus) error {
	if len(statuses) == 0 {
		_, err := fmt.Fprintln(output, "no addons found")
		return err
	}
	for index, status := range statuses {
		if index > 0 {
			fmt.Fprintln(output)
		}
		if err := renderAddonStatus(output, status); err != nil {
			return err
		}
	}
	return nil
}

func renderAddonStatus(output io.Writer, status addons.AddonStatus) error {
	fmt.Fprintf(output, "ADDON: %s\n", status.Name)
	fmt.Fprintf(output, "type: %s\n", status.Kind)
	if status.Entry.Source != "" {
		fmt.Fprintf(output, "source: %s\n", status.Entry.Source)
	}
	if status.Entry.WorktreeOf != "" {
		fmt.Fprintf(output, "worktree parent: %s\n", status.Entry.WorktreeOf)
	}
	if status.Entry.Branch != "" {
		fmt.Fprintf(output, "branch: %s\n", status.Entry.Branch)
	}
	fmt.Fprintf(output, "path: %s\n", status.Path)
	if status.PathAvailable {
		if status.Dirty {
			fmt.Fprintln(output, "repository: dirty")
		} else {
			fmt.Fprintln(output, "repository: clean")
		}
	} else {
		fmt.Fprintf(output, "repository: unavailable (%s)\n", status.RepositoryError)
	}
	if status.Kind == "worktree" {
		if status.ParentAvailable {
			fmt.Fprintln(output, "worktree parent status: available")
		} else {
			fmt.Fprintf(output, "worktree parent status: unavailable (%s)\n", status.ParentError)
		}
	}
	attached := strings.Join(status.AttachedProfiles, ", ")
	if attached == "" {
		attached = "none"
	}
	fmt.Fprintf(output, "attached profiles: %s\n", attached)
	return nil
}

func parseProfileExportArgs(args []string, profileName string) (string, string, error) {
	usage := "usage: lidoo profile export --name <profile> <file>"
	profileSet := strings.TrimSpace(profileName) != ""
	var positional []string
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--name":
			if profileSet || i+1 >= len(args) {
				return "", "", errors.New(usage)
			}
			i++
			profileName = args[i]
			profileSet = true
		case strings.HasPrefix(args[i], "--name="):
			if profileSet {
				return "", "", errors.New(usage)
			}
			profileName = strings.TrimPrefix(args[i], "--name=")
			profileSet = true
		case strings.HasPrefix(args[i], "-"):
			return "", "", fmt.Errorf("unknown profile export option %q", args[i])
		default:
			positional = append(positional, args[i])
		}
	}
	if !profileSet || strings.TrimSpace(profileName) == "" || len(positional) != 1 {
		return "", "", errors.New(usage)
	}
	return profileName, positional[0], nil
}

func parseProfileImportArgs(args []string, overwrite bool) (string, bool, error) {
	usage := "usage: lidoo profile import <file> [--yes]"
	var positional []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--yes", "-y":
			overwrite = true
		default:
			if strings.HasPrefix(args[i], "-") {
				return "", false, fmt.Errorf("unknown profile import option %q", args[i])
			}
			positional = append(positional, args[i])
		}
	}
	if len(positional) != 1 {
		return "", false, errors.New(usage)
	}
	return positional[0], overwrite, nil
}

func parseDBArgs(args []string, profileName, action string) (string, error) {
	profileName, _, err := parseDBValues(args, profileName, "", action, false)
	return profileName, err
}

func parseDBArgsWithDatabase(args []string, profileName, databaseName, action string) (string, string, error) {
	return parseDBValues(args, profileName, databaseName, action, true)
}

func parseDBValues(args []string, profileName, databaseName, action string, requireDatabase bool) (string, string, error) {
	usage := fmt.Sprintf("usage: lidoo db %s --name <profile>", action)
	if requireDatabase {
		usage += " --database <database>"
	}
	profileSet := strings.TrimSpace(profileName) != ""
	databaseSet := strings.TrimSpace(databaseName) != ""
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--name":
			if profileSet || i+1 >= len(args) {
				return "", "", errors.New(usage)
			}
			i++
			profileName = args[i]
			profileSet = true
		case strings.HasPrefix(args[i], "--name="):
			if profileSet {
				return "", "", errors.New(usage)
			}
			profileName = strings.TrimPrefix(args[i], "--name=")
			profileSet = true
		case args[i] == "--database":
			if !requireDatabase {
				return "", "", fmt.Errorf("unknown db %s option %q", action, args[i])
			}
			if databaseSet || i+1 >= len(args) {
				return "", "", errors.New(usage)
			}
			i++
			databaseName = args[i]
			databaseSet = true
		case strings.HasPrefix(args[i], "--database="):
			if !requireDatabase {
				return "", "", fmt.Errorf("unknown db %s option %q", action, args[i])
			}
			if databaseSet {
				return "", "", errors.New(usage)
			}
			databaseName = strings.TrimPrefix(args[i], "--database=")
			databaseSet = true
		default:
			return "", "", fmt.Errorf("unknown db %s option %q", action, args[i])
		}
	}
	if !profileSet || strings.TrimSpace(profileName) == "" || (requireDatabase && (!databaseSet || strings.TrimSpace(databaseName) == "")) {
		return "", "", errors.New(usage)
	}
	return profileName, databaseName, nil
}

func isProfileLifecycleCommand(command string) bool {
	switch command {
	case "run", "stop", "restart", "recreate", "remove":
		return true
	default:
		return false
	}
}

func confirmProfileRemoval(confirmation app.ProfileConfirmation) (bool, error) {
	_ = confirmation
	fmt.Fprint(os.Stderr, "container is running, stop it? [Y/N] ")
	var answer string
	if _, err := fmt.Fscan(os.Stdin, &answer); err != nil {
		return false, fmt.Errorf("read confirmation: %w", err)
	}
	return strings.EqualFold(answer, "y"), nil
}

func isDatabaseMutationCommand(command string) bool {
	switch command {
	case "init", "update", "drop", "backup", "restore":
		return true
	default:
		return false
	}
}

func commandMutatesState(command string, positional []string) bool {
	switch command {
	case "run", "stop", "restart", "recreate", "remove":
		return true
	case "addons":
		if len(positional) == 0 {
			return false
		}
		switch positional[0] {
		case "add", "attach", "detach", "rm", "worktree", "pull", "fetch":
			return true
		default:
			return false
		}
	case "profile":
		return len(positional) > 0 && positional[0] == "import"
	default:
		return false
	}
}

func changeAddonMounts(name string, addonNames []string, recreate, attach bool, state files.State) error {
	if err := docker.ValidateProfileName(name); err != nil {
		return err
	}
	previousState := files.CloneState(state)
	var err error
	if attach {
		err = addons.AttachToContainer(name, addonNames, state)
	} else {
		err = addons.DetachFromContainer(name, addonNames, state)
	}
	if err != nil {
		return err
	}

	exists, err := docker.ProfileExists(name)
	if err != nil {
		if recreate {
			return fmt.Errorf("inspect profile %q before recreation: %w", name, err)
		}
		fmt.Fprintf(os.Stderr, "warning: addon mounts for profile %q changed, but its container could not be inspected: %v; recreate it explicitly before they apply\n", name, err)
		return nil
	}
	if !exists {
		fmt.Printf("profile %q has no container; addon mounts apply when it is run\n", name)
		return nil
	}
	if !recreate {
		fmt.Printf("profile %q addon mounts changed; recreate it before they apply (or use --recreate)\n", name)
		return nil
	}

	fmt.Printf("recreating profile %q to apply addon mounts\n", name)
	if err := docker.Recreate(name, state); err != nil {
		files.RestoreState(state, previousState)
		return fmt.Errorf("recreate profile %q after addon change: %w", name, err)
	}
	return nil
}

func parseAddonArgs(args []string, profile, action string) (string, []string, bool, error) {
	usage := fmt.Sprintf("usage: lidoo addons %s --name <profile> <addon name> [<addon name> ...] [--recreate]", action)
	profileSet := strings.TrimSpace(profile) != ""
	var addonNames []string
	var recreate bool

	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--recreate":
			recreate = true
		case args[i] == "--name":
			if profileSet || i+1 >= len(args) {
				return "", nil, false, errors.New(usage)
			}
			i++
			profile = args[i]
			profileSet = true
		case strings.HasPrefix(args[i], "--name="):
			if profileSet {
				return "", nil, false, errors.New(usage)
			}
			profile = strings.TrimPrefix(args[i], "--name=")
			profileSet = true
		case strings.HasPrefix(args[i], "-"):
			return "", nil, false, fmt.Errorf("unknown addons %s option %q", action, args[i])
		default:
			addonNames = append(addonNames, args[i])
		}
	}

	if !profileSet || strings.TrimSpace(profile) == "" || len(addonNames) == 0 {
		return "", nil, false, errors.New(usage)
	}
	return profile, addonNames, recreate, nil
}

func parseAddonRemoveArgs(args []string, yes, force bool) (string, bool, bool, error) {
	usage := "usage: lidoo addons rm <addon name> [--yes] [--force]"
	var positional []string

	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--yes" || args[i] == "-y":
			yes = true
		case args[i] == "--force":
			force = true
		case strings.HasPrefix(args[i], "-"):
			return "", false, false, fmt.Errorf("unknown addons rm option %q", args[i])
		default:
			positional = append(positional, args[i])
		}
	}

	if len(positional) != 1 {
		return "", false, false, errors.New(usage)
	}
	return positional[0], yes, force, nil
}

func parseAddonAddArgs(args []string) (string, string, addons.AddOptions, error) {
	const usage = "usage: lidoo addons add <addon name> <git url> [--depth N] [--branch <branch>]"
	var positional []string
	var options addons.AddOptions
	var branchSet, depthSet bool

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--branch" || arg == "--depth":
			if i+1 >= len(args) {
				return "", "", options, errors.New(usage)
			}
			i++
			if arg == "--branch" {
				if branchSet || args[i] == "" || strings.HasPrefix(args[i], "-") {
					return "", "", options, errors.New(usage)
				}
				options.Branch = args[i]
				branchSet = true
			} else {
				if depthSet {
					return "", "", options, errors.New(usage)
				}
				depth, err := strconv.Atoi(args[i])
				if err != nil || depth < 1 {
					return "", "", options, errors.New(usage)
				}
				options.Depth = depth
				depthSet = true
			}
		case strings.HasPrefix(arg, "--branch="):
			if branchSet || strings.TrimPrefix(arg, "--branch=") == "" {
				return "", "", options, errors.New(usage)
			}
			options.Branch = strings.TrimPrefix(arg, "--branch=")
			branchSet = true
		case strings.HasPrefix(arg, "--depth="):
			if depthSet {
				return "", "", options, errors.New(usage)
			}
			depth, err := strconv.Atoi(strings.TrimPrefix(arg, "--depth="))
			if err != nil || depth < 1 {
				return "", "", options, errors.New(usage)
			}
			options.Depth = depth
			depthSet = true
		case strings.HasPrefix(arg, "-"):
			return "", "", options, fmt.Errorf("unknown addons add option %q", arg)
		default:
			positional = append(positional, arg)
		}
	}
	if len(positional) != 2 || (options.Branch != "" && (strings.TrimSpace(options.Branch) != options.Branch || strings.HasPrefix(options.Branch, "-"))) {
		return "", "", options, errors.New(usage)
	}
	return positional[0], positional[1], options, nil
}

func parseWorktreeArgs(args []string) (string, string, string, bool, error) {
	var positional []string
	var branch string
	var yes bool
	branchSet := false

	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--branch":
			if branchSet || i+1 >= len(args) {
				return "", "", "", false, errors.New("usage: lidoo addons worktree <source> <name> --branch <branch>")
			}
			i++
			branch = args[i]
			branchSet = true
		case strings.HasPrefix(args[i], "--branch="):
			if branchSet {
				return "", "", "", false, errors.New("usage: lidoo addons worktree <source> <name> --branch <branch>")
			}
			branch = strings.TrimPrefix(args[i], "--branch=")
			branchSet = true
		case args[i] == "--yes" || args[i] == "-y":
			yes = true
		case strings.HasPrefix(args[i], "-"):
			return "", "", "", false, fmt.Errorf("unknown addons worktree option %q", args[i])
		default:
			positional = append(positional, args[i])
		}
	}

	if len(positional) != 2 || !branchSet {
		return "", "", "", false, errors.New("usage: lidoo addons worktree <source> <name> --branch <branch>")
	}
	return positional[0], positional[1], branch, yes, nil
}

func validatePositionals(command string, positional []string) error {
	switch command {
	case "backup":
		if len(positional) > 1 {
			return errors.New("usage: lidoo backup --name <profile> --database <database> [options] [<destination>]")
		}
	case "restore":
		if len(positional) != 1 {
			return errors.New("usage: lidoo restore --name <profile> --database <database> [options] <source>")
		}
	case "tui", "list", "status", "logs", "init", "update", "drop", "run", "wait", "recreate", "stop", "restart", "remove":
		if len(positional) != 0 {
			return fmt.Errorf("%s does not accept positional arguments", command)
		}
	case "db":
		if len(positional) == 0 {
			return errors.New("usage: lidoo db list --name <profile>")
		}
	}
	return nil
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: lidoo <tui|list|status|logs|init|update|drop|backup|restore|run|wait|recreate|stop|restart|remove|db> [options]")
	fmt.Fprintln(os.Stderr, "  tui")
	fmt.Fprintln(os.Stderr, "  list")
	fmt.Fprintln(os.Stderr, "  status [--name <profile>]")
	fmt.Fprintln(os.Stderr, "  logs --name <profile> [--follow] [--tail N]")
	fmt.Fprintln(os.Stderr, "  wait --name <profile> [--timeout <duration>]")
	fmt.Fprintln(os.Stderr, "  init --name <profile> --database <database> [--modules <csv>]")
	fmt.Fprintln(os.Stderr, "  update --name <profile> --database <database> [--update-all]")
	fmt.Fprintln(os.Stderr, "  drop --name <profile> --database <database> --yes")
	fmt.Fprintln(os.Stderr, "  backup --name <profile> --database <database> [options] [<destination>]")
	fmt.Fprintln(os.Stderr, "  restore --name <profile> --database <database> [--copy|--move] [--force] [--neutralize] [--jobs N] <source>")
	fmt.Fprintln(os.Stderr, "  run|stop|restart|remove --name <profile>")
	fmt.Fprintln(os.Stderr, "  db list --name <profile>")
	fmt.Fprintln(os.Stderr, "  db info --name <profile> --database <database>")
	fmt.Fprintln(os.Stderr, "  db shell --name <profile> --database <database>")
	fmt.Fprintln(os.Stderr, "  profile export --name <profile> <file>")
	fmt.Fprintln(os.Stderr, "  profile import <file> [--yes]")
	fmt.Fprintln(os.Stderr, "       lidoo addons list")
	fmt.Fprintln(os.Stderr, "       lidoo addons status [<addon name>]")
	fmt.Fprintln(os.Stderr, "       lidoo addons add <addon name> <git url> [--depth N] [--branch <branch>]")
	fmt.Fprintln(os.Stderr, "       lidoo addons rm <addon name> [--yes] [--force]")
	fmt.Fprintln(os.Stderr, "       lidoo addons worktree <source> <name> --branch <branch> [--yes]")
	fmt.Fprintln(os.Stderr, "       lidoo addons fetch <addon name>")
	fmt.Fprintln(os.Stderr, "       lidoo addons pull <addon name>")
	fmt.Fprintln(os.Stderr, "       lidoo addons attach --name <profile> <addon name> [<addon name> ...] [--recreate]")
	fmt.Fprintln(os.Stderr, "       lidoo addons detach --name <profile> <addon name> [<addon name> ...] [--recreate]")
}
