package main

import (
	"fmt"
	"os"

	"lidoo/internal/addons"
	"lidoo/internal/docker"
	"lidoo/internal/files"
)

func main() {
	workspace, err := files.ReadWorkspace()
	if err != nil {
		fmt.Fprintln(os.Stderr, "read workspace:", err)
		os.Exit(1)
	}

	exitCode := 0
	if len(os.Args) < 2 {
		usage()
		exitCode = 2
	} else {
		switch os.Args[1] {
		case "up":
			err = docker.Up(os.Args[2:])
		case "stop":
			err = docker.Stop(os.Args[2:])
		case "restart":
			err = docker.Restart(os.Args[2:])
		case "remove":
			err = docker.Remove(os.Args[2:])
		case "addons":
			err = addons.Run(os.Args[2:], workspace)
		default:
			usage()
			exitCode = 2
		}
	}

	if saveErr := files.SaveWorkspace(workspace); saveErr != nil {
		fmt.Fprintln(os.Stderr, "save workspace:", saveErr)
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
	fmt.Fprintln(os.Stderr, "usage: lidoo <up|stop|restart|remove> --name <container name> [--version <odoo version>]")
	fmt.Fprintln(os.Stderr, "       lidoo addons add <addon name> <git url>")
}
