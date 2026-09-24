package addons

import (
	"errors"
	"fmt"
	"strings"

	"lidoo/internal/files"
	"lidoo/internal/profile"
)

func DetachFromContainer(container string, names []string, state files.State) error {
	return DetachFromContainerWithOptions(container, names, state, OperationOptions{})
}

func DetachFromContainerWithOptions(container string, names []string, state files.State, operationOptions OperationOptions) error {
	operationOptions = normalizeOperationOptions(operationOptions)
	if err := operationOptions.Context.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(container) == "" {
		return errors.New("container name cannot be empty")
	}
	if len(names) == 0 {
		return errors.New("at least one addon is required")
	}
	for _, name := range names {
		if !validAddonName(name) {
			return fmt.Errorf("invalid addon name %q", name)
		}
	}

	config, found, err := profile.Lookup(state, container)
	if err != nil {
		return fmt.Errorf("update container %q: %w", container, err)
	}
	if !found {
		return fmt.Errorf("update container %q: container %q not found", container, container)
	}

	remove := make(map[string]bool, len(names))
	for _, name := range names {
		remove[name] = true
	}
	remaining := make([]string, 0, len(config.Addons))
	for _, addon := range config.Addons {
		if !remove[addon] {
			remaining = append(remaining, addon)
		}
	}
	config.Addons = remaining
	if err := profile.Put(state, container, config); err != nil {
		return fmt.Errorf("update container %q: %w", container, err)
	}
	return nil
}
