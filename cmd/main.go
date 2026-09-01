package main

import (
	"fmt"
	"os"

	"lidoo/internal/docker"
	"lidoo/internal/hosts"
)

func main() {
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
	case "up":
		err = docker.Up(os.Args[2:])
	case "stop":
		err = docker.Stop(os.Args[2:])
	case "restart":
		err = docker.Restart(os.Args[2:])
	case "remove":
		err = docker.Remove(os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: lidoo <list|up|stop|restart|remove> [options]")
	fmt.Fprintln(os.Stderr, "  list")
	fmt.Fprintln(os.Stderr, "  up|stop|restart|remove --name <profile>")
	os.Exit(2)
}
