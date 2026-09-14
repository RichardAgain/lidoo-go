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
	if err := ValidateProfileName(name); err != nil {
		return err
	}
	exists, err := ProfileExists(name)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("no profile with name %q", name)
	}
	running, err := containerIsRunning(name)
	if err != nil {
		return err
	}
	if running {
		if err := Stop(name); err != nil {
			return fmt.Errorf("stop container %q: %w", name, err)
		}
	}
	previousState := files.CloneState(state)
	if err := RemoveWithState(name, false, state); err != nil {
		return fmt.Errorf("remove container %q: %w", name, err)
	}
	files.RestoreState(state, previousState)
	if err := Run(name, "", state); err != nil {
		return fmt.Errorf("recreate container %q: %w", name, err)
	}
	return nil
}
