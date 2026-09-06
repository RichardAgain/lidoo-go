package docker

import (
	"errors"
	"fmt"

	"lidoo/internal/files"
)

func Recreate(name string, state files.State) error {
	if name == "" {
		return errors.New("recreate requires container name")
	}

	if err := Stop(name); err != nil {
		return fmt.Errorf("stop container %q: %w", name, err)
	}
	if err := Remove(name, false); err != nil {
		return fmt.Errorf("remove container %q: %w", name, err)
	}
	if err := Run(name, "", state); err != nil {
		return fmt.Errorf("recreate container %q: %w", name, err)
	}
	return nil
}
