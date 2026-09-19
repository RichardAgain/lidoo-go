package docker

import (
	"errors"
	"fmt"

	"lidoo/internal/files"
)

func Recreate(name string, state files.State) error {
	return RecreateWithOptions(name, state, CommandOptions{})
}

func RecreateWithOptions(name string, state files.State, options CommandOptions) error {
	options = normalizeCommandOptions(options)
	if name == "" {
		return errors.New("recreate requires container name")
	}
	if err := ValidateProfileName(name); err != nil {
		return err
	}
	exists, err := ProfileExistsWithOptions(name, options)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("no profile with name %q", name)
	}
	running, err := containerIsRunningWithOptions(name, options)
	if err != nil {
		return err
	}
	if err := networkExistsWithContext(options.Context); err != nil {
		return err
	}
	if running {
		if err := StopWithOptions(name, options); err != nil {
			return fmt.Errorf("stop container %q: %w", name, err)
		}
	}
	previousState := files.CloneState(state)
	if err := RemoveWithStateOptions(name, state, RemoveOptions{
		CommandOptions: options,
		Yes:            true,
	}); err != nil {
		return fmt.Errorf("remove container %q: %w", name, err)
	}
	files.RestoreState(state, previousState)
	if err := RunWithOptions(name, "", state, options); err != nil {
		return fmt.Errorf("recreate container %q: %w", name, err)
	}
	return nil
}
