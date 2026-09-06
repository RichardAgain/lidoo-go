package main

import (
	"fmt"
	"os"

	"lidoo/internal/addons"
	"lidoo/internal/docker"
	"lidoo/internal/files"
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
		exitCode = 2
	} else {
		switch os.Args[1] {
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
	fmt.Fprintln(os.Stderr, "usage: lidoo <run|recreate|stop|restart|remove> --name <container name> [--version <odoo version>]")
	fmt.Fprintln(os.Stderr, "       lidoo addons add <addon name> <git url>")
	fmt.Fprintln(os.Stderr, "       lidoo <container> addons add <addon name> [<addon name> ...]")
}
