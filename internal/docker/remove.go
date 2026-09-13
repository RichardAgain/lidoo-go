package docker

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"lidoo/internal/files"
	"lidoo/internal/proxy"
)

// Remove keeps the original package API for callers that do not have loaded
// workspace state. The CLI uses RemoveWithState so routing can be updated
// before the container disappears.
func Remove(name string, yes bool) error {
	return remove(name, yes, nil)
}

func RemoveWithState(name string, yes bool, state files.State) error {
	return remove(name, yes, state)
}

func remove(name string, yes bool, state files.State) error {
	if name == "" {
		return errors.New("remove requires name")
	}
	if err := ValidateProfileName(name); err != nil {
		return err
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

	var previousState files.State
	if state != nil {
		previousState = cloneState(state)
		if err := files.RemoveContainer(state, name); err != nil {
			return fmt.Errorf("update workspace: %w", err)
		}
		if err := proxy.Sync(state); err != nil {
			restoreState(state, previousState)
			return fmt.Errorf("synchronize Caddy routing: %w", err)
		}
	}

	rollback := func(primary error) error {
		if state == nil {
			return primary
		}
		restoreState(state, previousState)
		return combineErrors(primary, proxy.Sync(state))
	}

	for _, container := range running {
		if err := dockerQuiet("stop", container); err != nil {
			return rollback(fmt.Errorf("stop container %s: %w", container, err))
		}
	}
	for _, container := range containers {
		if err := dockerQuiet("rm", container); err != nil {
			return rollback(fmt.Errorf("remove container %s: %w", container, err))
		}
	}
	return nil
}
