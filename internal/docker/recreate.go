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
	previousState := cloneState(state)
	if err := RemoveWithState(name, false, state); err != nil {
		return fmt.Errorf("remove container %q: %w", name, err)
	}
	restoreState(state, previousState)
	if err := Run(name, "", state); err != nil {
		return fmt.Errorf("recreate container %q: %w", name, err)
	}
	return nil
}
