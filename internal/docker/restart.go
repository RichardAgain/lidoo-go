package docker

import (
	"errors"
	"flag"
	"fmt"
	"os"
)

func Restart(args []string) error {
	flags := flag.NewFlagSet("restart", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	name := flags.String("name", "", "container name")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("restart does not accept positional arguments")
	}
	if *name == "" {
		return errors.New("restart requires --name")
	}

	containers, err := containerIDs("label="+containerNameLabel+"="+*name, false)
	if err != nil {
		return fmt.Errorf("find container with name %q: %w", *name, err)
	}
	if len(containers) == 0 {
		return fmt.Errorf("no running container with name %q", *name)
	}
	for _, container := range containers {
		if err := dockerQuiet("restart", container); err != nil {
			return fmt.Errorf("restart container %s: %w", container, err)
		}
	}
	return nil
}
