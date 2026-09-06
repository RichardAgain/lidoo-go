package docker

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"lidoo/internal/hosts"
)

func Remove(name string, yes bool) error {
	if name == "" {
		return errors.New("remove requires name")
	}

	containers, err := containerIDs("label="+containerNameLabel+"="+name, true)
	if err != nil {
		return fmt.Errorf("find container with name %q: %w", name, err)
	}
	if len(containers) == 0 {
		return fmt.Errorf("no container with name %q", name)
	}

	running, err := containerIDs("label="+containerNameLabel+"="+name, false)
	if err != nil {
		return fmt.Errorf("check container with name %q: %w", name, err)
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
		if err := dockerQuiet("stop", container); err != nil {
			return fmt.Errorf("stop container %s: %w", container, err)
		}
	}
	for _, container := range containers {
		if err := dockerQuiet("rm", container); err != nil {
			return fmt.Errorf("remove container %s: %w", container, err)
		}
	}
	if err := hosts.Remove(profileHostname(name)); err != nil {
		return err
	}
	return nil
}
