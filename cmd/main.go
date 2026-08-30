package main

import (
	"fmt"
	"os"

	"lidoo/internal/docker"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}

	var err error
	switch os.Args[1] {
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
	fmt.Fprintln(os.Stderr, "usage: lidoo <up|stop|restart|remove> --name <container name> [--version <odoo version>]")
	os.Exit(2)
}
