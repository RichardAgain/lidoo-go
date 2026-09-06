package main

import (
	"fmt"
	"os"

	"lidoo/internal/addons"
	"lidoo/internal/docker"
	"lidoo/internal/files"
	"lidoo/internal/hosts"
)

func main() {
	state, err := files.ReadState()
	if err != nil {
		fmt.Fprintln(os.Stderr, "read state:", err)
		os.Exit(1)
	}

	exitCode := 0
	if len(os.Args) < 2 {
		usage()
	}
	if os.Args[1] == hosts.ElevatedCommand {
		if err := hosts.RunElevated(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	var err error
	switch os.Args[1] {
		case "list":
			err = docker.List(os.Args[2:])
		case "init":
			err = docker.Init(os.Args[2:])
		case "update":
			err = docker.Update(os.Args[2:])
		case "drop":
			err = docker.Drop(os.Args[2:])
		case "up":
			err = docker.Up(os.Args[2:])
		case "stop":
			err = docker.Stop(os.Args[2:])
		case "restart":
			err = docker.Restart(os.Args[2:])
		case "remove":
			err = docker.Remove(os.Args[2:])
		case "run":
			err = docker.Run(os.Args[2:], state)
		case "recreate":
			err = docker.Recreate(os.Args[2:], state)
		case "stop":
			err = docker.Stop(os.Args[2:])
		case "restart":
			err = docker.Restart(os.Args[2:])
		case "remove":
			err = docker.Remove(os.Args[2:])
		case "addons":
			err = addons.Run(os.Args[2:], state)
		default:
			if len(os.Args) >= 5 && os.Args[2] == "addons" {
				err = addons.RunForContainer(os.Args[1:], state)
			} else {
				usage()
				exitCode = 2
			}
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

func usage() {
	fmt.Fprintln(os.Stderr, "usage: lidoo <list|init|update|drop|run|recreate|stop|restart|remove> [options]")
	fmt.Fprintln(os.Stderr, "  list")
	fmt.Fprintln(os.Stderr, "  init --name <profile> --database <database> [--modules <csv>]")
	fmt.Fprintln(os.Stderr, "  update --name <profile> --database <database> [--update-all]")
	fmt.Fprintln(os.Stderr, "  drop --name <profile> --database <database> --yes")
	fmt.Fprintln(os.Stderr, "  up|stop|restart|remove --name <profile>")
	fmt.Fprintln(os.Stderr, "       lidoo addons add <addon name> <git url>")
	fmt.Fprintln(os.Stderr, "       lidoo <container> addons add <addon name> [<addon name> ...]")
}
