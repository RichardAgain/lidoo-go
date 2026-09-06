package docker

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"lidoo/internal/files"
)

func Recreate(args []string, state files.State) error {
	flags := flag.NewFlagSet("recreate", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	name := flags.String("name", "", "container name")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("recreate does not accept positional arguments")
	}
	if *name == "" {
		return errors.New("recreate requires container name")
	}

	containerArgs := []string{"--name", *name}
	if err := Stop(containerArgs); err != nil {
		return fmt.Errorf("stop container %q: %w", *name, err)
	}
	if err := Remove(containerArgs); err != nil {
		return fmt.Errorf("remove container %q: %w", *name, err)
	}
	if err := Run(args, state); err != nil {
		return fmt.Errorf("recreate container %q: %w", *name, err)
	}
	return nil
}
