package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"lidoo/internal/addons"
	"lidoo/internal/docker"
	"lidoo/internal/files"
	"lidoo/internal/hosts"
	"lidoo/internal/odoo"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	command := os.Args[1]
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(os.Stderr)

	var name, version, database, modules string
	var updateAll, yes bool

	flags.StringVar(&name, "name", "", "container name")
	flags.StringVar(&version, "version", "", "Odoo version")
	flags.StringVar(&database, "database", "", "database name")
	flags.StringVar(&modules, "modules", "base", "comma-separated modules to install")
	flags.BoolVar(&updateAll, "update-all", false, "force a complete module update")
	flags.BoolVar(&yes, "y", false, "confirm to all")
	flags.BoolVar(&yes, "yes", false, "confirm to all")

	if err := flags.Parse(os.Args[2:]); err != nil {
		os.Exit(2)
	}
	positional := flags.Args()

	if command == hosts.ElevatedCommand {
		if len(positional) != 2 {
			fmt.Fprintln(os.Stderr, "hosts write requires add/remove and hostname")
			os.Exit(2)
		}
		if err := hosts.RunElevated(positional[0], positional[1]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	state, err := files.ReadState()
	if err != nil {
		fmt.Fprintln(os.Stderr, "read state:", err)
		os.Exit(1)
	}

	exitCode := 0
	if commandRejectsPositionals(command) {
		if err := noPositionals(command, positional); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
	}

	switch command {
	case "list":
		err = docker.List()
	case "init":
		err = odoo.Init(name, database, modules, state)
	case "update":
		err = odoo.Update(name, database, updateAll, state)
	case "drop":
		err = odoo.Drop(name, database, yes, state)
	case "run":
		err = docker.Run(name, version, state)
	case "recreate":
		err = docker.Recreate(name, state)
	case "stop":
		err = docker.Stop(name)
	case "restart":
		err = docker.Restart(name)
	case "remove":
		err = docker.Remove(name, yes)
	case "addons":
		switch {
		case len(positional) > 0 && positional[0] == "add":
			if len(positional) != 3 {
				err = errors.New("usage: lidoo addons add <addon name> <git url>")
			} else {
				err = addons.Add(positional[1], positional[2], state)
			}
		case len(positional) > 0 && positional[0] == "attach":
			var profile string
			var addonNames []string
			profile, addonNames, err = parseAddonArgs(positional[1:], name, "attach")
			if err == nil {
				err = addons.AttachToContainer(profile, addonNames, state)
			}
		case len(positional) > 0 && positional[0] == "detach":
			var profile string
			var addonNames []string
			profile, addonNames, err = parseAddonArgs(positional[1:], name, "detach")
			if err == nil {
				err = addons.DetachFromContainer(profile, addonNames, state)
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
		default:
			err = errors.New("usage: lidoo addons add <addon name> <git url> | lidoo addons attach|detach --name <profile> <addon name> [<addon name> ...] | lidoo addons worktree <source> <name> --branch <branch> [--yes]")
		}
	default:
		usage()
		exitCode = 2
	}

	if saveErr := files.SaveState(state); saveErr != nil {
		fmt.Fprintln(os.Stderr, "save state:", saveErr)
		exitCode = 1
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		exitCode = 1
	}
	if exitCode != 0 {
		os.Exit(exitCode)
	}
}

func parseAddonArgs(args []string, profile, action string) (string, []string, error) {
	usage := fmt.Sprintf("usage: lidoo addons %s --name <profile> <addon name> [<addon name> ...]", action)
	profileSet := strings.TrimSpace(profile) != ""
	var addonNames []string

	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--name":
			if profileSet || i+1 >= len(args) {
				return "", nil, errors.New(usage)
			}
			i++
			profile = args[i]
			profileSet = true
		case strings.HasPrefix(args[i], "--name="):
			if profileSet {
				return "", nil, errors.New(usage)
			}
			profile = strings.TrimPrefix(args[i], "--name=")
			profileSet = true
		case strings.HasPrefix(args[i], "-"):
			return "", nil, fmt.Errorf("unknown addons %s option %q", action, args[i])
		default:
			addonNames = append(addonNames, args[i])
		}
	}

	if !profileSet || strings.TrimSpace(profile) == "" || len(addonNames) == 0 {
		return "", nil, errors.New(usage)
	}
	return profile, addonNames, nil
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

func commandRejectsPositionals(command string) bool {
	switch command {
	case "list", "init", "update", "drop", "run", "recreate", "stop", "restart", "remove":
		return true
	default:
		return false
	}
}

func noPositionals(command string, positional []string) error {
	if len(positional) != 0 {
		return fmt.Errorf("%s does not accept positional arguments", command)
	}
	return nil
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: lidoo <list|init|update|drop|run|recreate|stop|restart|remove> [options]")
	fmt.Fprintln(os.Stderr, "  list")
	fmt.Fprintln(os.Stderr, "  init --name <profile> --database <database> [--modules <csv>]")
	fmt.Fprintln(os.Stderr, "  update --name <profile> --database <database> [--update-all]")
	fmt.Fprintln(os.Stderr, "  drop --name <profile> --database <database> --yes")
	fmt.Fprintln(os.Stderr, "  run|stop|restart|remove --name <profile>")
	fmt.Fprintln(os.Stderr, "       lidoo addons add <addon name> <git url>")
	fmt.Fprintln(os.Stderr, "       lidoo addons rm <addon name> [--yes] [--force]")
	fmt.Fprintln(os.Stderr, "       lidoo addons worktree <source> <name> --branch <branch> [--yes]")
	fmt.Fprintln(os.Stderr, "       lidoo addons attach --name <profile> <addon name> [<addon name> ...]")
	fmt.Fprintln(os.Stderr, "       lidoo addons detach --name <profile> <addon name> [<addon name> ...]")
}
