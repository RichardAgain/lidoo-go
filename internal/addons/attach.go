package addons

import (
	"errors"
	"fmt"
	"strings"

	"lidoo/internal/files"
	"lidoo/internal/profile"
)

func AttachToContainer(container string, names []string, state files.State) error {
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

	created, err := profile.Ensure(state, container)
	if err != nil {
		return fmt.Errorf("add container %q to state: %w", container, err)
	}
	if created {
		fmt.Printf("\033[32mstate entry for container %q created\033[0m\n", container)
	}

	config, _, err := profile.Lookup(state, container)
	if err != nil {
		return fmt.Errorf("update container %q: %w", container, err)
	}
	seen := make(map[string]bool, len(config.Addons))
	for _, name := range config.Addons {
		seen[name] = true
	}
	for _, name := range names {
		if !seen[name] {
			config.Addons = append(config.Addons, name)
			seen[name] = true
		}
	}
	if err := profile.Put(state, container, config); err != nil {
		return fmt.Errorf("update container %q: %w", container, err)
	}
	return nil
}
