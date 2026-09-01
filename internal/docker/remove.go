package docker

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"lidoo/internal/hosts"
)

func Remove(args []string) error {
	flags := flag.NewFlagSet("remove", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	name := flags.String("name", "", "container name")
	var yes bool
	flags.BoolVar(&yes, "y", false, "confirm stopping running containers")
	flags.BoolVar(&yes, "yes", false, "confirm stopping running containers")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("remove does not accept positional arguments")
	}
	if *name == "" {
		return errors.New("remove requires --name")
	}

	containers, err := containerIDs("label="+containerNameLabel+"="+*name, true)
	if err != nil {
		return fmt.Errorf("find container with name %q: %w", *name, err)
	}
	if len(containers) == 0 {
		return fmt.Errorf("no container with name %q", *name)
	}

	running, err := containerIDs("label="+containerNameLabel+"="+*name, false)
	if err != nil {
		return fmt.Errorf("check container with name %q: %w", *name, err)
	}
	if len(running) > 0 && !yes {
		fmt.Fprint(os.Stderr, "container is running, stop it? [Y/N] ")
		var answer string
		if _, err := fmt.Fscan(os.Stdin, &answer); err != nil {
			return fmt.Errorf("read confirmation: %w", err)
		}
		if !strings.EqualFold(answer, "y") {
			return nil
		}
	}
	for _, container := range running {
		if err := docker("stop", container); err != nil {
			return fmt.Errorf("stop container %s: %w", container, err)
		}
	}
	for _, container := range containers {
		if err := docker("rm", container); err != nil {
			return fmt.Errorf("remove container %s: %w", container, err)
		}
	}
	if err := hosts.Remove(profileHostname(*name)); err != nil {
		return err
	}
	return nil
}
