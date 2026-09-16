package docker

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"lidoo/internal/files"
	"lidoo/internal/profile"
	"lidoo/internal/proxy"
)

// RemoveOptions controls output and confirmation for profile removal. Confirm
// is called only when a running container must be stopped. It belongs to the
// presentation boundary; the Docker package only supplies the decision point.
type RemoveOptions struct {
	CommandOptions
	Yes     bool
	Confirm func() (bool, error)
}

// Remove keeps the original package API for callers that do not have loaded
// workspace state. The CLI uses the application service for new operations.
func Remove(name string, yes bool) error {
	return remove(name, nil, RemoveOptions{
		CommandOptions: CommandOptions{},
		Yes:            yes,
		Confirm:        defaultRemovalConfirmation,
	})
}

// RemoveWithState keeps the original package API for compatibility.
func RemoveWithState(name string, yes bool, state files.State) error {
	return remove(name, state, RemoveOptions{
		CommandOptions: CommandOptions{},
		Yes:            yes,
		Confirm:        defaultRemovalConfirmation,
	})
}

// RemoveWithStateOptions removes a profile using caller-owned terminal
// handling and process context.
func RemoveWithStateOptions(name string, state files.State, options RemoveOptions) error {
	return remove(name, state, options)
}

func remove(name string, state files.State, options RemoveOptions) error {
	options.CommandOptions = normalizeCommandOptions(options.CommandOptions)
	if name == "" {
		return errors.New("remove requires name")
	}
	if err := ValidateProfileName(name); err != nil {
		return err
	}

	containers, err := containerIDsWithOptions("label="+containerNameLabel+"="+name, true, options.CommandOptions)
	if err != nil {
		return fmt.Errorf("find container with name %q: %w", name, err)
	}
	if len(containers) == 0 {
		return fmt.Errorf("no container with name %q", name)
	}

	running, err := containerIDsWithOptions("label="+containerNameLabel+"="+name, false, options.CommandOptions)
	if err != nil {
		return fmt.Errorf("check container with name %q: %w", name, err)
	}
	if len(running) > 0 && !options.Yes {
		if options.Confirm == nil {
			return errors.New("remove requires confirmation for a running container")
		}
		confirmed, err := options.Confirm()
		if err != nil {
			return err
		}
		if !confirmed {
			return nil
		}
	}

	var previousState files.State
	if state != nil {
		previousState = files.CloneState(state)
		if err := profile.Remove(state, name); err != nil {
			return fmt.Errorf("update workspace: %w", err)
		}
		if err := proxy.SyncWithContext(options.Context, state); err != nil {
			files.RestoreState(state, previousState)
			return fmt.Errorf("synchronize Caddy routing: %w", err)
		}
	}

	rollback := func(primary error) error {
		if state == nil {
			return primary
		}
		files.RestoreState(state, previousState)
		return combineErrors(primary, proxy.SyncWithContext(options.Context, state))
	}

	for _, container := range running {
		if err := dockerQuietWithOptions(options.CommandOptions, "stop", container); err != nil {
			return rollback(fmt.Errorf("stop container %s: %w", container, err))
		}
	}
	for _, container := range containers {
		if err := dockerQuietWithOptions(options.CommandOptions, "rm", container); err != nil {
			return rollback(fmt.Errorf("remove container %s: %w", container, err))
		}
	}
	return nil
}

func defaultRemovalConfirmation() (bool, error) {
	fmt.Fprint(os.Stderr, "container is running, stop it? [Y/N] ")
	var answer string
	if _, err := fmt.Fscan(os.Stdin, &answer); err != nil {
		return false, fmt.Errorf("read confirmation: %w", err)
	}
	return strings.EqualFold(answer, "y"), nil
}
