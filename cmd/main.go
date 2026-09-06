package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"lidoo/internal/addons"
	"lidoo/internal/docker"
	"lidoo/internal/files"
	"lidoo/internal/hosts"
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
		err = docker.Init(name, database, modules)
	case "update":
		err = docker.Update(name, database, updateAll)
	case "drop":
		err = docker.Drop(name, database, yes)
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
		if len(positional) != 3 || positional[0] != "add" {
			err = errors.New("usage: lidoo addons add <addon name> <git url>")
		} else {
			err = addons.Add(positional[1], positional[2], state)
		}
	default:
		if len(positional) >= 3 && positional[0] == "addons" && positional[1] == "add" {
			err = addons.AddToContainer(command, positional[2:], state)
		} else {
			usage()
			exitCode = 2
		}
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
	fmt.Fprintln(os.Stderr, "       lidoo <container> addons add <addon name> [<addon name> ...]")
}
